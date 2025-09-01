package taskbus

import (
	"context"
	"math"
	"time"
)

// Message 为统一消息结构。
type Message struct {
	Topic   string
	Key     string
	Body    []byte
	Headers map[string]string
}

// Handler 处理 MQ 消息。
type Handler func(ctx context.Context, msg Message) error

// RetryPolicy 定义重试策略。
type RetryPolicy interface {
	NextBackoff(attempt int) (time.Duration, bool)
}

// Producer 统一发布接口。
type Producer interface {
	Publish(ctx context.Context, msg Message) error
	PublishDelay(ctx context.Context, msg Message, delay time.Duration) error
}

// Consumer 统一消费接口。
type Consumer interface {
	// Consume 订阅 topic，group 为消费组；返回停止函数。
	Consume(ctx context.Context, topic, group string, handler Handler, mws ...Middleware) (stop func(context.Context) error, err error)
}

// MQ 聚合 Producer 与 Consumer，并暴露 Close 以释放资源。
type MQ interface {
	Producer
	Consumer
	Close(ctx context.Context) error
}

// ExponentialBackoff 简单指数回退策略。
type ExponentialBackoff struct {
	Base       time.Duration
	Factor     float64
	MaxRetries int
}

func (e ExponentialBackoff) NextBackoff(attempt int) (time.Duration, bool) {
	if attempt >= e.MaxRetries {
		return 0, false
	}
	d := time.Duration(float64(e.Base) * math.Pow(e.Factor, float64(attempt)))
	return d, true
}

// ---- no-op 默认实现，确保包可编译运行 ----

type noopMQ struct{}

func newNoopMQ() MQ { return noopMQ{} }

func (noopMQ) Publish(ctx context.Context, msg Message) error                           { return nil }
func (noopMQ) PublishDelay(ctx context.Context, msg Message, delay time.Duration) error { return nil }
func (noopMQ) Consume(ctx context.Context, topic, group string, handler Handler, mws ...Middleware) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
func (noopMQ) Close(ctx context.Context) error { return nil }
