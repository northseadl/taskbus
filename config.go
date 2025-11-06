package taskbus

import (
	"time"
)

// DelayMode 用于 RabbitMQ 延时消息兼容模式。
type DelayMode string

const (
	DelayModeStandard DelayMode = "standard" // 使用 x-delayed-message 插件（x-delay）
	DelayModeAliyun   DelayMode = "aliyun"   // 使用阿里云原生（delay）
)

// Config 为包总配置，应用通过 New 传入。
type Config struct {
	// Namespace 服务命名空间：用于隔离 Jobs/Cron/EventBus 等组件的 topic/锁键前缀
	// 建议与服务名一致，例如："chuanggo-core"、"chuanggo-intelligence"
	Namespace string
	MQ        MQConfig
	Job       JobConfig
	Cron      CronConfig
	EventBus  EventBusConfig
	// Stream 为独立于 Namespace 的全局分布式订阅配置（一个实例一个独立 MQ）
	Stream StreamConfig
	Logger LoggerConfig

	// Idempotency 可选配置：若提供 KV/Redis，则默认启用 Jobs 与 EventBus 的幂等检查。
	Idempotency IdempotencyConfig
}

type MQProvider string

const (
	MQProviderRabbitMQ MQProvider = "rabbitmq"
	MQProviderRedis    MQProvider = "redis"
)

type MQConfig struct {
	Provider MQProvider
	RabbitMQ RabbitMQConfig
	Redis    RedisConfig
	// 统一重试策略配置，适用于 MQ 层、EventBus、Jobs
	Retry RetryConfig
}

type RabbitMQConfig struct {
	URI                 string
	Exchange            string
	DelayedExchange     string
	Prefetch            int
	ConsumerConcurrency int
	// DelayMode 选择延时消息兼容模式；默认 standard。
	DelayMode DelayMode
}

type RedisConfig struct {
	Addr                string
	Username            string
	Password            string
	DB                  int
	ConsumerConcurrency int
}

type JobConfig struct {
	Retry           RetryConfig
	DeadLetterTopic string
	// GroupPrefix 已弃用；请使用 Config.Namespace 控制隔离。
	GroupPrefix string
	// DefaultGroup 当调用方未指定消费组或传入空字符串时使用的默认组名。
	DefaultGroup string
}

type RetryConfig struct {
	Base       time.Duration
	Factor     float64
	MaxRetries int
}

type CronConfig struct {
	Timezone string
	// Distributed 开启分布式调度（Scheduler + Executor）。
	Distributed bool
	// LeaderLockKey 分布式锁键（Redis）。
	LeaderLockKey string
	// LeaderTTL 锁过期时间。
	LeaderTTL time.Duration
	// ExecutorGroup 执行器消费组。
	ExecutorGroup string
}

type EventBusConfig struct {
	// Mode 事件总线模式：isolated（基于 namespace 的隔离）或 global（跨服务共享）
	Mode EventBusMode
	// SubscriberConcurrency 订阅者并发度（默认 1）
	SubscriberConcurrency int
}

// EventBusMode 定义事件总线模式
type EventBusMode string

const (
	EventBusModeIsolated EventBusMode = "isolated"
	EventBusModeGlobal   EventBusMode = "global"
)

// StreamConfig 管理多个独立的 Stream 实例（每个实例绑定到一个独立的 MQ 连接或 Redis DB）
type StreamConfig struct {
	Streams map[string]StreamInstanceConfig
}

// StreamInstanceConfig 单个 Stream 的 MQ 配置
type StreamInstanceConfig struct {
	Provider MQProvider
	RabbitMQ RabbitMQConfig
	Redis    RedisConfig
}

type LoggerConfig struct {
	Level string
}
