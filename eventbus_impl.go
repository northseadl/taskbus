package taskbus

import (
	"context"
	"sync"
)

type bus struct{ c *client }

func newBus(c *client) EventBus { return &bus{c: c} }

func (b *bus) Publish(ctx context.Context, e Event) error {
	headers := copyHeaders(e.Metadata)
	if headers == nil {
		headers = map[string]string{}
	}
	if e.Type != "" {
		headers["type"] = e.Type
	}
	// 根据模式对 topic 前缀化（除 Stream 外的组件统一加 taskbus 前缀）
	var topic string
	switch b.c.cfg.EventBus.Mode {
	case EventBusModeGlobal:
		topic = "taskbus.event." + e.Topic
	default: // isolated
		topic = "taskbus." + b.c.namespace + ".event." + e.Topic
	}
	msg := Message{Topic: topic, Key: e.Subject, Body: e.Payload, Headers: headers}
	return b.c.mq.Publish(ctx, msg)
}

func (b *bus) Subscribe(topic, group string, filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) (func(context.Context) error, error) {
	// 包装为 MQ Handler
	fn := func(ctx context.Context, m Message) error {
		// 解码原始业务 topic（去除前缀）
		rawTopic := m.Topic
		if b.c != nil {
			isoPrefix := "taskbus." + b.c.namespace + ".event."
			globPrefix := "taskbus.event."
			switch b.c.cfg.EventBus.Mode {
			case EventBusModeGlobal:
				if len(rawTopic) > len(globPrefix) && rawTopic[:len(globPrefix)] == globPrefix {
					rawTopic = rawTopic[len(globPrefix):]
				}
			default:
				if len(rawTopic) > len(isoPrefix) && rawTopic[:len(isoPrefix)] == isoPrefix {
					rawTopic = rawTopic[len(isoPrefix):]
				}
			}
		}
		e := Event{Topic: rawTopic, Subject: m.Key, Payload: m.Body, Metadata: m.Headers}
		if m.Headers != nil {
			if t, ok := m.Headers["type"]; ok {
				e.Type = t
			}
		}
		if filter != nil && !filter(e) {
			return nil
		}
		return handler(ctx, e)
	}
	// 将 EventMiddleware 转为 MQ Middleware
	var q []Middleware
	for i := range mws {
		mw := mws[i]
		q = append(q, func(next Handler) Handler {
			return func(ctx context.Context, m Message) error {
				e := Event{Topic: m.Topic, Subject: m.Key, Payload: m.Body, Metadata: m.Headers}
				if m.Headers != nil {
					if t, ok := m.Headers["type"]; ok {
						e.Type = t
					}
				}
				wrapped := func(ctx context.Context, ev Event) error { return next(ctx, m) }
				return mw(wrapped)(ctx, e)
			}
		})
	}
	// 订阅时对 topic 做同样的前缀化
	subTopic := topic
	switch b.c.cfg.EventBus.Mode {
	case EventBusModeGlobal:
		subTopic = "taskbus.event." + topic
	default:
		subTopic = "taskbus." + b.c.namespace + ".event." + topic
	}
	return b.c.mq.Consume(context.Background(), subTopic, group, fn, q...)
}

// 简单过滤器辅助
func FilterByType(t string) Filter { return func(e Event) bool { return e.Type == t } }

// 并发控制（预留）
type subGroup struct{ wg sync.WaitGroup }
