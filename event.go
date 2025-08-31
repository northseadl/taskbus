package taskbus

import "context"

// Event 领域事件。
type Event struct {
	Topic    string
	Type     string
	Subject  string
	Metadata map[string]string
	Payload  []byte
}

type Filter func(e Event) bool

// EventBus 提供事件发布与订阅。
type EventBus interface {
	Publish(ctx context.Context, e Event) error
	Subscribe(topic, group string, filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) (stop func(context.Context) error, err error)
}

// ---- no-op 实现 ----

type noopBus struct{}

func newNoopBus() EventBus { return noopBus{} }

func (noopBus) Publish(ctx context.Context, e Event) error { return nil }
func (noopBus) Subscribe(topic, group string, filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

