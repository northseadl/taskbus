package taskbus

import "context"

// Event 是领域事件结构。
//
// Fields:
//   - Topic: 事件主题（如 "user.created"）
//   - Type: 事件类型（用于过滤，如 "UserCreated"）
//   - Subject: 事件主体标识（如用户 ID）
//   - Metadata: 事件元数据
//   - Payload: 事件载荷（通常是 JSON）
type Event struct {
	Topic    string            // 事件主题
	Type     string            // 事件类型
	Subject  string            // 事件主体
	Metadata map[string]string // 元数据
	Payload  []byte            // 载荷
}

// Filter 是事件过滤器函数，返回 true 表示接受该事件。
type Filter func(e Event) bool

// EventBus 提供事件发布与订阅功能。
//
// Example:
//
//	// 发布事件
//	cli.Bus().Publish(ctx, taskbus.Event{
//	    Topic:   "user.created",
//	    Type:    "UserCreated",
//	    Subject: "user-123",
//	    Payload: []byte(`{"name": "Alice"}`),
//	})
//
//	// 订阅事件
//	stop, _ := cli.Bus().Subscribe("user.#", "my-service",
//	    taskbus.FilterByType("UserCreated"),
//	    func(ctx context.Context, e taskbus.Event) error {
//	        log.Println("User created:", e.Subject)
//	        return nil
//	    },
//	)
type EventBus interface {
	// Publish 发布事件。
	Publish(ctx context.Context, e Event) error

	// Subscribe 订阅事件。
	// topic 支持通配符（如 "user.#" 匹配所有 user 事件）。
	// group 为消费组名称。
	// filter 为可选过滤器，返回 false 的事件将被跳过。
	// 返回 stop 函数用于优雅停止订阅。
	Subscribe(topic, group string, filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) (stop func(context.Context) error, err error)
}

// ---- no-op 实现 ----

type noopBus struct{}

func newNoopBus() EventBus { return noopBus{} }

func (noopBus) Publish(ctx context.Context, e Event) error { return nil }
func (noopBus) Subscribe(topic, group string, filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
