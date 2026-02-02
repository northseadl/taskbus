package taskbus

import (
	"context"
)

// bus 实现 EventBus 接口，提供事件发布与订阅。
type bus struct{ c *client }

func newBus(c *client) EventBus { return &bus{c: c} }

// globalEventPrefix 全局模式的事件前缀。
const globalEventPrefix = "taskbus.event."

// eventTopicPrefix 返回当前模式下的事件 topic 前缀。
func (b *bus) eventTopicPrefix() string {
	switch b.c.cfg.EventBus.Mode {
	case EventBusModeGlobal:
		return globalEventPrefix
	default: // isolated
		return buildTopicPrefix(b.c.namespace, "event")
	}
}

// buildEventTopic 构建完整的事件 topic。
func (b *bus) buildEventTopic(topic string) string {
	return b.eventTopicPrefix() + topic
}

// extractEventTopic 从完整 topic 中提取业务 topic。
func (b *bus) extractEventTopic(fullTopic string) string {
	return trimTopicPrefix(fullTopic, b.eventTopicPrefix())
}

// Publish 发布事件。
func (b *bus) Publish(ctx context.Context, e Event) error {
	headers := copyHeaders(e.Metadata)
	if headers == nil {
		headers = map[string]string{}
	}
	if e.Type != "" {
		headers["type"] = e.Type
	}
	topic := b.buildEventTopic(e.Topic)
	msg := Message{Topic: topic, Key: e.Subject, Body: e.Payload, Headers: headers}
	return b.c.mq.Publish(ctx, msg)
}

// Subscribe 订阅事件。
func (b *bus) Subscribe(topic, group string, filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) (func(context.Context) error, error) {
	// 构建最终的 handler
	finalHandler := b.buildEventHandler(filter, handler, mws...)

	// 订阅时对 topic 做前缀化
	subTopic := b.buildEventTopic(topic)

	concurrency := b.c.cfg.EventBus.SubscriberConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	stops := make([]func(context.Context) error, 0, concurrency)
	for i := 0; i < concurrency; i++ {
		stop, err := b.c.mq.Consume(context.Background(), subTopic, group, finalHandler)
		if err != nil {
			for _, s := range stops {
				_ = s(context.Background())
			}
			return nil, err
		}
		stops = append(stops, stop)
	}
	return func(ctx context.Context) error {
		for _, s := range stops {
			_ = s(ctx)
		}
		return nil
	}, nil
}

// buildEventHandler 构建带中间件的事件处理器。
func (b *bus) buildEventHandler(filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) Handler {
	// 最终的事件处理函数
	finalEventHandler := handler

	// 逆序包装中间件
	for i := len(mws) - 1; i >= 0; i-- {
		finalEventHandler = mws[i](finalEventHandler)
	}

	// 返回 MQ Handler
	return func(ctx context.Context, m Message) error {
		e := b.messageToEvent(m)
		if filter != nil && !filter(e) {
			return nil
		}
		return finalEventHandler(ctx, e)
	}
}

// messageToEvent 将 Message 转换为 Event。
func (b *bus) messageToEvent(m Message) Event {
	rawTopic := b.extractEventTopic(m.Topic)
	e := Event{
		Topic:    rawTopic,
		Subject:  m.Key,
		Payload:  m.Body,
		Metadata: m.Headers,
	}
	if m.Headers != nil {
		if t, ok := m.Headers["type"]; ok {
			e.Type = t
		}
	}
	return e
}

// FilterByType 返回一个按事件类型过滤的 Filter。
func FilterByType(t string) Filter {
	return func(e Event) bool { return e.Type == t }
}
