package taskbus

import "context"

// withDefaultJobMiddleware 将默认中间件接入 Jobs 执行链。
func withDefaultJobMiddleware(j Jobs, m JobMiddleware) Jobs {
	type wrapped struct{ Jobs }
	// 装饰 StartWorkers 注入默认中间件
	return wrapped{Jobs: jobWithMiddleware{j, m}}
}

type jobWithMiddleware struct {
	Jobs
	mw JobMiddleware
}

func (w jobWithMiddleware) StartWorkers(ctx context.Context, groups map[string]int, mws ...JobMiddleware) (func(context.Context) error, error) {
	// 默认中间件追加到调用者中间件之前，避免覆盖调用者定制行为
	mws = append([]JobMiddleware{w.mw}, mws...)
	return w.Jobs.StartWorkers(ctx, groups, mws...)
}

// withDefaultBusMiddleware 将默认 MQ 中间件接入 EventBus 消费链。
func withDefaultBusMiddleware(b EventBus, m Middleware) EventBus {
	type wrapped struct{ EventBus }
	return wrapped{EventBus: busWithMiddleware{b, m}}
}

type busWithMiddleware struct {
	EventBus
	mw Middleware
}

func (w busWithMiddleware) Subscribe(topic, group string, filter Filter, handler func(context.Context, Event) error, mws ...EventMiddleware) (func(context.Context) error, error) {
	// 将 MQ Middleware 注入到 EventMiddleware 链首（转换后生效）
	mw := w.mw
	conv := EventMiddleware(func(next func(context.Context, Event) error) func(context.Context, Event) error {
		return func(ctx context.Context, e Event) error {
			m := Message{Topic: e.Topic, Key: e.Subject, Body: e.Payload, Headers: e.Metadata}
			return mw(func(ctx context.Context, msg Message) error { return next(ctx, e) })(ctx, m)
		}
	})
	mws = append([]EventMiddleware{conv}, mws...)
	return w.EventBus.Subscribe(topic, group, filter, handler, mws...)
}

