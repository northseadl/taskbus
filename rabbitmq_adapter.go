package taskbus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// rabbitMQAdapter 实现 MQ 接口。仅处理发布与消费，失败重试由上层（Jobs/EventBus）中间件管理。
// 延时发布可使用 x-delayed-message 插件（standard），或阿里云 delay 头（aliyun）。

type rabbitMQAdapter struct {
	cfg    RabbitMQConfig
	retry  RetryConfig
	logger Logger
	mode   DelayMode

	conn   *amqp.Connection
	connMu sync.Mutex
}

func newRabbitMQAdapterWithMode(cfg RabbitMQConfig, mode DelayMode, retry RetryConfig, logger Logger) (MQ, error) {
	if cfg.URI == "" || cfg.Exchange == "" {
		return nil, fmt.Errorf("rabbitmq config invalid")
	}
	if mode == DelayModeStandard && cfg.DelayedExchange == "" {
		return nil, fmt.Errorf("delayed exchange required in standard mode")
	}
	// dd dddddd
	if retry.Base <= 0 {
		retry.Base = time.Second
	}
	if retry.Factor <= 0 {
		retry.Factor = 2.0
	}
	ad := &rabbitMQAdapter{cfg: cfg, mode: mode, retry: retry, logger: logger}
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
	// amqp.Dial 自动支持 amqp:// 和 amqps://
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
	r.logger.Info(context.Background(), "declare exchange", "exchange", r.cfg.Exchange)
	if err := ch.ExchangeDeclare(r.cfg.Exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	// 延时交换机仅在 standard 模式下声明
	if r.mode == DelayModeStandard {
		if r.cfg.DelayedExchange == "" {
			return fmt.Errorf("delayed exchange required in standard mode")
		}
		args := amqp.Table{"x-delayed-type": "topic"}
		r.logger.Info(context.Background(), "declare delayed exchange", "exchange", r.cfg.DelayedExchange)
		if err := ch.ExchangeDeclare(r.cfg.DelayedExchange, "x-delayed-message", true, false, false, false, args); err != nil {
			return err
		}
	}
	return nil
}

func (r *rabbitMQAdapter) Publish(ctx context.Context, msg Message) error {
	if err := r.ensureConnection(); err != nil {
		return fmt.Errorf("rabbitmq connection failed: %w", err)
	}
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq channel creation failed: %w", err)
	}
	defer ch.Close()
	
	// 监听 Channel 关闭和消息退回（用于检测阿里云 Serverless 的特殊错误）
	closeChan := ch.NotifyClose(make(chan *amqp.Error, 1))
	rets := ch.NotifyReturn(make(chan amqp.Return, 1))
	
	if r.cfg.Prefetch > 0 {
		_ = ch.Qos(r.cfg.Prefetch, 0, false)
	}
	
	err = ch.PublishWithContext(ctx, r.cfg.Exchange, msg.Topic, true, false, amqp.Publishing{
		ContentType: "application/octet-stream",
		MessageId:   msg.Key,
		Timestamp:   time.Now(),
		Headers:     stringMapToTable(msg.Headers),
		Body:        msg.Body,
	})
	if err != nil {
		return fmt.Errorf("rabbitmq publish failed (topic=%s): %w", msg.Topic, err)
	}
	
	// 检查是否有立即错误（Channel 关闭或消息无法路由）
	select {
	case ret := <-rets:
		return fmt.Errorf("message unroutable to topic %s: %s (code=%d)", msg.Topic, ret.ReplyText, ret.ReplyCode)
	case closeErr := <-closeChan:
		return fmt.Errorf("channel closed immediately after publish: %w", closeErr)
	case <-time.After(100 * time.Millisecond):
		// 发布成功，没有立即错误
	}
	return err
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
		rets := ch.NotifyReturn(make(chan amqp.Return, 1))
		err = ch.PublishWithContext(ctx, r.cfg.Exchange, msg.Topic, true, false, amqp.Publishing{
			ContentType: "application/octet-stream",
			MessageId:   msg.Key,
			Timestamp:   time.Now(),
			Headers:     headers,
			Body:        msg.Body,
		})
		if err != nil {
			return err
		}
		select {
		case ret := <-rets:
			r.logger.Error(ctx, "mq return (unroutable)", "exchange", ret.Exchange, "routing_key", ret.RoutingKey, "code", ret.ReplyCode, "text", ret.ReplyText)
		default:
		}
		return nil
	}
	// standard: 发布到延时交换机，使用 x-delay
	headers["x-delay"] = ms
	rets := ch.NotifyReturn(make(chan amqp.Return, 1))
	err = ch.PublishWithContext(ctx, r.cfg.DelayedExchange, msg.Topic, true, false, amqp.Publishing{
		ContentType: "application/octet-stream",
		MessageId:   msg.Key,
		Timestamp:   time.Now(),
		Headers:     headers,
		Body:        msg.Body,
	})
	if err != nil {
		return err
	}
	select {
	case ret := <-rets:
		r.logger.Error(ctx, "mq return (unroutable)", "exchange", ret.Exchange, "routing_key", ret.RoutingKey, "code", ret.ReplyCode, "text", ret.ReplyText)
	default:
	}
	return nil
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
	r.logger.Info(ctx, "queue bind", "queue", q.Name, "exchange", r.cfg.Exchange, "binding_key", topic)
	if err := ch.QueueBind(q.Name, topic, r.cfg.Exchange, false, nil); err != nil {
		ch.Close()
		return nil, err
	}
	// 绑定到延时交换机（延时消息），仅在 standard 模式
	if r.mode == DelayModeStandard {
		r.logger.Info(ctx, "queue bind", "queue", q.Name, "exchange", r.cfg.DelayedExchange, "binding_key", topic)
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

	// 监听 Channel 关闭
	closeChan := ch.NotifyClose(make(chan *amqp.Error, 1))

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
			case err := <-closeChan:
				// Channel 被服务器关闭（如阿里云 Serverless 的 406/504 错误）
				r.logger.Error(ctx, "rabbitmq channel closed by server", "queue", q.Name, "error", err.Error())
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
						// 失败：统一应用层延迟重投（先发布后确认），发布失败则 Nack 重投
						attempt := 0
						if s, ok := m.Headers["x-retry-count"]; ok {
							if n, e := strconv.Atoi(s); e == nil {
								attempt = n
							}
						}
						nextAttempt := attempt + 1
						if nextAttempt <= r.retry.MaxRetries {
							h := copyHeaders(m.Headers)
							h["x-retry-count"] = strconv.Itoa(nextAttempt)
							delay := time.Duration(float64(r.retry.Base) * math.Pow(r.retry.Factor, float64(nextAttempt-1)))
							if err := r.PublishDelay(ctx, Message{Topic: m.Topic, Key: m.Key, Body: m.Body, Headers: h}, delay); err == nil {
								_ = del.Ack(false)
								return
							}
						}
						// 超过最大重试：ACK；发布失败：Nack 重投
						if nextAttempt > r.retry.MaxRetries {
							_ = del.Ack(false)
						} else {
							_ = del.Nack(false, true)
						}
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
