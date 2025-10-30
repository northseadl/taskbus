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
	MQ       MQConfig
	Job      JobConfig
	Cron     CronConfig
	EventBus EventBusConfig
	Logger   LoggerConfig

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
	// GroupPrefix 用于在消费组名前追加统一前缀，实现跨服务隔离。
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
	SubscriberConcurrency int
}

type LoggerConfig struct {
	Level string
}
