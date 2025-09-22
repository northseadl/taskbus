package taskbus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// rabbitMQAdapter 实现 MQ 接口。仅处理发布与消费，失败重试由上层（Jobs/EventBus）中间件管理。
// 延时发布可使用 x-delayed-message 插件（standard），或阿里云 delay 头（aliyun）。

type rabbitMQAdapter struct {
	cfg    RabbitMQConfig
	logger Logger
	mode   DelayMode

	conn   *amqp.Connection
	connMu sync.Mutex
}

func newRabbitMQAdapterWithMode(cfg RabbitMQConfig, mode DelayMode, logger Logger) (MQ, error) {
	if cfg.URI == "" || cfg.Exchange == "" {
		return nil, fmt.Errorf("rabbitmq config invalid")
	}
	if mode == DelayModeStandard && cfg.DelayedExchange == "" {
		return nil, fmt.Errorf("delayed exchange required in standard mode")
	}
	ad := &rabbitMQAdapter{cfg: cfg, mode: mode, logger: logger}
	if err := ad.ensureConnection(); err != nil {
		return nil, err
	}
	if err := ad.declareTopology(); err != nil {
		return nil, err
	}
	return ad, nil
}

func (r *rabbitMQAdapter) ensureConnection() error {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	if r.conn != nil && !r.conn.IsClosed() {
		return nil
	}
	conn, err := amqp.Dial(r.cfg.URI)
	if err != nil {
		return err
	}
	r.conn = conn
	return nil
}

func (r *rabbitMQAdapter) declareTopology() error {
	ch, err := r.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	// 普通 topic exchange
	if err := ch.ExchangeDeclare(r.cfg.Exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	// 延时交换机仅在 standard 模式下声明
	if r.mode == DelayModeStandard {
		if r.cfg.DelayedExchange == "" {
			return fmt.Errorf("delayed exchange required in standard mode")
		}
		args := amqp.Table{"x-delayed-type": "topic"}
		if err := ch.ExchangeDeclare(r.cfg.DelayedExchange, "x-delayed-message", true, false, false, false, args); err != nil {
			return err
		}
	}
	return nil
}

func (r *rabbitMQAdapter) Publish(ctx context.Context, msg Message) error {
	if err := r.ensureConnection(); err != nil {
		return err
	}
	ch, err := r.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if r.cfg.Prefetch > 0 {
		_ = ch.Qos(r.cfg.Prefetch, 0, false)
	}
	return ch.PublishWithContext(ctx, r.cfg.Exchange, msg.Topic, false, false, amqp.Publishing{
		ContentType: "application/octet-stream",
		MessageId:   msg.Key,
		Timestamp:   time.Now(),
		Headers:     stringMapToTable(msg.Headers),
		Body:        msg.Body,
	})
}

func (r *rabbitMQAdapter) PublishDelay(ctx context.Context, msg Message, delay time.Duration) error {
	if err := r.ensureConnection(); err != nil {
		return err
	}
	ch, err := r.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if r.cfg.Prefetch > 0 {
		_ = ch.Qos(r.cfg.Prefetch, 0, false)
	}
	headers := stringMapToTable(msg.Headers)
	if headers == nil {
		headers = amqp.Table{}
	}
	ms := int64(delay / time.Millisecond)
	if r.mode == DelayModeAliyun {
		// 直接发布到普通交换机，使用 delay 字段
		headers["delay"] = fmt.Sprintf("%d", ms)
		return ch.PublishWithContext(ctx, r.cfg.Exchange, msg.Topic, false, false, amqp.Publishing{
			ContentType: "application/octet-stream",
			MessageId:   msg.Key,
			Timestamp:   time.Now(),
			Headers:     headers,
			Body:        msg.Body,
		})
	}
	// standard: 发布到延时交换机，使用 x-delay
	headers["x-delay"] = ms
	return ch.PublishWithContext(ctx, r.cfg.DelayedExchange, msg.Topic, false, false, amqp.Publishing{
		ContentType: "application/octet-stream",
		MessageId:   msg.Key,
		Timestamp:   time.Now(),
		Headers:     headers,
		Body:        msg.Body,
	})
}

func (r *rabbitMQAdapter) Consume(ctx context.Context, topic, group string, handler Handler, mws ...Middleware) (func(context.Context) error, error) {
	if err := r.ensureConnection(); err != nil {
		return nil, err
	}
	ch, err := r.conn.Channel()
	if err != nil {
		return nil, err
	}
	// 注意：关闭在 stop 时处理
	if r.cfg.Prefetch > 0 {
		_ = ch.Qos(r.cfg.Prefetch, 0, false)
	}
	qName := fmt.Sprintf("%s-%s", sanitizeQueueName(topic), sanitizeQueueName(group))
	q, err := ch.QueueDeclare(qName, true, false, false, false, amqp.Table{})
	if err != nil {
		ch.Close()
		return nil, err
	}
	// 绑定到普通交换机（即时消息）
	if err := ch.QueueBind(q.Name, topic, r.cfg.Exchange, false, nil); err != nil {
		ch.Close()
		return nil, err
	}
	// 绑定到延时交换机（延时消息），仅在 standard 模式
	if r.mode == DelayModeStandard {
		if err := ch.QueueBind(q.Name, topic, r.cfg.DelayedExchange, false, nil); err != nil {
			ch.Close()
			return nil, err
		}
	}
	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		ch.Close()
		return nil, err
	}

	// 组装中间件
	final := handler
	for i := len(mws) - 1; i >= 0; i-- {
		final = mws[i](final)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		concurrency := r.cfg.ConsumerConcurrency
		if concurrency <= 0 {
			concurrency = 1
		}
		wg := &sync.WaitGroup{}
		sem := make(chan struct{}, concurrency)
		for {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case d, ok := <-msgs:
				if !ok {
					wg.Wait()
					return
				}
				sem <- struct{}{}
				wg.Add(1)
				go func(del amqp.Delivery) {
					defer func() { <-sem; wg.Done() }()
					m := Message{Topic: del.RoutingKey, Key: del.MessageId, Body: del.Body, Headers: tableToStringMap(del.Headers)}
					if err := final(ctx, m); err != nil {
						r.logger.Error(ctx, "consumer handler error", "topic", m.Topic, "err", err)
						_ = del.Nack(false, true)
					} else {
						_ = del.Ack(false)
					}
				}(d)
			}
		}
	}()

	stop := func(sctx context.Context) error {
		// 关闭 channel 触发 msgs 退出
		if err := ch.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			return err
		}
		select {
		case <-done:
			return nil
		case <-sctx.Done():
			return sctx.Err()
		}
	}
	return stop, nil
}

func (r *rabbitMQAdapter) Close(ctx context.Context) error {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

func stringMapToTable(m map[string]string) amqp.Table {
	if len(m) == 0 {
		return nil
	}
	t := amqp.Table{}
	for k, v := range m {
		t[k] = v
	}
	return t
}

func tableToStringMap(t amqp.Table) map[string]string {
	if len(t) == 0 {
		return nil
	}
	m := make(map[string]string, len(t))
	for k, v := range t {
		switch vv := v.(type) {
		case string:
			m[k] = vv
		case int32, int64, int:
			m[k] = fmt.Sprintf("%v", vv)
		}
	}
	return m
}

func sanitizeQueueName(s string) string {
	// 简化处理：避免包含不合法字符
	forbidden := []rune{' ', '*', '#', '/'}
	out := []rune{}
	for _, r := range s {
		skip := false
		for _, f := range forbidden {
			if r == f {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "q"
	}
	return string(out)
}
