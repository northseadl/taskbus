package taskbus

import (
	"context"
	"math"
	"time"
)

// Message 是统一的消息结构，用于 MQ 发布和消费。
//
// Fields:
//   - Topic: 消息主题/路由键，用于 MQ 路由
//   - Key: 业务键，可用于幂等检查或分区路由
//   - Body: 消息体（二进制）
//   - Headers: 消息头元数据
type Message struct {
	Topic   string            // 消息主题
	Key     string            // 业务键（幂等/分区）
	Body    []byte            // 消息体
	Headers map[string]string // 消息头
}

// Handler 是 MQ 消息处理函数签名。
// 返回 nil 表示处理成功，返回 error 将触发重试逻辑。
type Handler func(ctx context.Context, msg Message) error

// RetryPolicy 定义重试策略接口。
type RetryPolicy interface {
	// NextBackoff 返回第 attempt 次重试的等待时间。
	// 如果 ok 为 false，表示不再重试。
	NextBackoff(attempt int) (delay time.Duration, ok bool)
}

// Producer 是消息发布接口。
type Producer interface {
	// Publish 发布即时消息。
	Publish(ctx context.Context, msg Message) error

	// PublishDelay 发布延时消息，delay 后才会被消费。
	PublishDelay(ctx context.Context, msg Message, delay time.Duration) error
}

// Consumer 是消息消费接口。
type Consumer interface {
	// Consume 订阅 topic，使用 group 作为消费组。
	// 返回 stop 函数用于优雅停止消费。
	// mws 为可选的中间件链。
	Consume(ctx context.Context, topic, group string, handler Handler, mws ...Middleware) (stop func(context.Context) error, err error)
}

// MQ 是统一的消息队列接口，聚合 Producer 和 Consumer。
// 实现包括 RabbitMQ 和 Redis Streams。
type MQ interface {
	Producer
	Consumer

	// Close 优雅关闭 MQ 连接，释放资源。
	Close(ctx context.Context) error
}

// ExponentialBackoff 实现指数退避重试策略。
//
// 计算公式: delay = Base * (Factor ^ attempt)
//
// Example:
//
//	backoff := ExponentialBackoff{Base: time.Second, Factor: 2, MaxRetries: 3}
//	// attempt 0: 1s, attempt 1: 2s, attempt 2: 4s
type ExponentialBackoff struct {
	Base       time.Duration // 基础等待时间
	Factor     float64       // 退避因子
	MaxRetries int           // 最大重试次数
}

// NextBackoff 实现 RetryPolicy 接口。
func (e ExponentialBackoff) NextBackoff(attempt int) (time.Duration, bool) {
	if attempt >= e.MaxRetries {
		return 0, false
	}
	d := time.Duration(float64(e.Base) * math.Pow(e.Factor, float64(attempt)))
	return d, true
}

// ---- no-op 默认实现 ----

type noopMQ struct{}

func newNoopMQ() MQ { return noopMQ{} }

func (noopMQ) Publish(ctx context.Context, msg Message) error { return nil }
func (noopMQ) PublishDelay(ctx context.Context, msg Message, delay time.Duration) error {
	return nil
}
func (noopMQ) Consume(ctx context.Context, topic, group string, handler Handler, mws ...Middleware) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
func (noopMQ) Close(ctx context.Context) error { return nil }
