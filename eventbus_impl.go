package taskbus

import (
	"context"
	"sync"
)

type bus struct { c *client }

func newBus(c *client) EventBus { return &bus{c: c} }

func (b *bus) Publish(ctx context.Context, e Event) error {
	headers := copyHeaders(e.Metadata)
	if headers == nil { headers = map[string]string{} }
	if e.Type != "" { headers["type"] = e.Type }
	msg := Message{Topic: e.Topic, Key: e.Subject, Body: e.Payload, Headers: headers}
	return b.c.mq.Publish(ctx, msg)
}

func (b *bus) Subscribe(topic, group string, filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) (func(context.Context) error, error) {
	// 包装为 MQ Handler
	fn := func(ctx context.Context, m Message) error {
		e := Event{Topic: m.Topic, Subject: m.Key, Payload: m.Body, Metadata: m.Headers}
		if m.Headers != nil { if t, ok := m.Headers["type"]; ok { e.Type = t } }
		if filter != nil && !filter(e) { return nil }
		return handler(ctx, e)
	}
	// 将 EventMiddleware 转为 MQ Middleware
	var q []Middleware
	for i := range mws {
		mw := mws[i]
		q = append(q, func(next Handler) Handler {
			return func(ctx context.Context, m Message) error {
				e := Event{Topic: m.Topic, Subject: m.Key, Payload: m.Body, Metadata: m.Headers}
				if m.Headers != nil { if t, ok := m.Headers["type"]; ok { e.Type = t } }
				wrapped := func(ctx context.Context, ev Event) error { return next(ctx, m) }
				return mw(wrapped)(ctx, e)
			}
		})
	}
	return b.c.mq.Consume(context.Background(), topic, group, fn, q...)
}

// 简单过滤器辅助
func FilterByType(t string) Filter { return func(e Event) bool { return e.Type == t } }

// 并发控制（预留）
type subGroup struct { wg sync.WaitGroup }

