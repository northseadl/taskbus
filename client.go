package taskbus

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client 对外统一入口，聚合 MQ/Jobs/Cron/EventBus。
// 通过 New 构造，按配置选择具体适配器与策略。
// 所有方法要求调用方传递 context 控制超时/取消。
//
// 线程安全：实现需保障并发安全。

type Client interface {
	// Start 启动后台协程等资源（如延时调度器）。
	Start(ctx context.Context) error
	// Close 优雅关闭，等待在途任务，遵循 ctx 超时。
	Close(ctx context.Context) error

	// MQ 暴露统一的消息队列抽象。
	MQ() MQ
	// Jobs 暴露任务队列（基于 MQ）。
	Jobs() Jobs
	// Cron 暴露 Cron 调度。
	Cron() Cron
	// Bus 暴露事件总线。
	Bus() EventBus
	// Streams 暴露 Stream 管理器（全局分布式订阅，独立 MQ 实例）
	Streams() StreamManager
}

// New 创建 Client 实例。
func New(ctx context.Context, cfg Config, opts ...Option) (Client, error) {
	// 校验并应用默认配置
	if cfg.Namespace == "" {
		// 未配置则回退到默认命名空间，避免 Stream/MQ-only 用法报错
		cfg.Namespace = "default"
	}
	if !isValidNamespace(cfg.Namespace) {
		// 非法时回退默认，避免启动失败
		cfg.Namespace = "default"
	}
	applyDefaultConfig(&cfg)

	c := &client{
		cfg:       cfg,
		logger:    defaultLogger{},
		namespace: cfg.Namespace,
	}
	if c.cfg.Job.DefaultGroup == "" {
		c.cfg.Job.DefaultGroup = "default"
	}
	for _, opt := range opts {
		opt(c)
	}

	// 根据 Provider 装配 MQ
	switch cfg.MQ.Provider {
	case MQProviderRabbitMQ:
		mode := cfg.MQ.RabbitMQ.DelayMode
		if mode == "" {
			mode = DelayModeStandard
		}
		mq, err := newRabbitMQAdapterWithMode(cfg.MQ.RabbitMQ, mode, cfg.MQ.Retry, c.logger)
		if err != nil {
			return nil, err
		}
		c.mq = mq
	case MQProviderRedis:
		mq, err := newRedisAdapter(cfg.MQ.Redis, cfg.MQ.Retry, c.logger)
		if err != nil {
			return nil, err
		}
		c.mq = mq
	default:
		c.mq = newNoopMQ()
	}

	// 幂等中间件集成（可选启用）：提供 KV 或 Redis 参数即开启
	var mqIdem Middleware
	var jobIdem JobMiddleware
	if cfg.Idempotency.KV != nil || cfg.Idempotency.RedisAddr != "" {
		idemCfg := cfg.Idempotency
		// 如果未提供 KV 尝试用 Redis 参数构建
		if idemCfg.KV == nil && idemCfg.RedisAddr != "" {
			idemCfg.KV = RedisKV{R: redis.NewClient(&redis.Options{Addr: idemCfg.RedisAddr, Username: idemCfg.RedisUsername, Password: idemCfg.RedisPassword, DB: idemCfg.RedisDB})}
		}
		mqIdem = NewIdempotencyMiddleware(idemCfg)
		// Job 默认 key 策略：优先 Message.Key；否则使用 payload 作为业务唯一键
		jobIdem = NewJobIdempotencyMiddleware(IdempotencyConfig{
			KV: idemCfg.KV, Prefix: idemCfg.Prefix, TTL: idemCfg.TTL,
			KeyFunc: func(ctx context.Context, m Message) (string, error) {
				if m.Key != "" {
					return m.Key, nil
				}
				return string(m.Body), nil
			},
		})
	}

	c.jobs = newJobs(c)
	c.cron = newCron(c)
	c.bus = newBus(c)

	// 初始化 Stream 管理器
	sm, err := newStreamManager(cfg, c.logger)
	if err != nil {
		return nil, err
	}
	c.streams = sm

	// 将幂等中间件装配到默认栈：Jobs、EventBus（若启用）
	if jobIdem != nil {
		c.jobs = withDefaultJobMiddleware(c.jobs, jobIdem)
	}
	if mqIdem != nil {
		c.bus = withDefaultBusMiddleware(c.bus, mqIdem)
	}
	return c, nil
}

type client struct {
	cfg    Config
	logger Logger

	mq   MQ
	jobs Jobs
	cron Cron
	bus  EventBus
	// namespace 用于前缀化 topic 与锁键
	namespace string
	// streams 独立的 Stream 实例集合
	streams StreamManager
}

func (c *client) Start(ctx context.Context) error { return nil }
func (c *client) Close(ctx context.Context) error {
	if c.mq != nil {
		_ = c.mq.Close(ctx)
	}
	if c.streams != nil {
		_ = c.streams.Close(ctx)
	}
	return nil
}
func (c *client) MQ() MQ                 { return c.mq }
func (c *client) Jobs() Jobs             { return c.jobs }
func (c *client) Cron() Cron             { return c.cron }
func (c *client) Bus() EventBus          { return c.bus }
func (c *client) Streams() StreamManager { return c.streams }

// Option 允许注入替换默认行为（如 Logger）。
type Option func(*client)

// WithLogger 注入自定义日志实现。
func WithLogger(l Logger) Option {
	return func(c *client) {
		if l != nil {
			c.logger = l
		}
	}
}

// --- defaults & validation ---

func isValidNamespace(ns string) bool {
	matched, _ := regexp.MatchString(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, ns)
	return matched
}

func applyDefaultConfig(cfg *Config) {
	// Job
	if cfg.Job.DefaultGroup == "" {
		cfg.Job.DefaultGroup = "default"
	}
	if cfg.Job.Retry.Base <= 0 {
		cfg.Job.Retry.Base = 1 * time.Second
	}
	if cfg.Job.Retry.Factor <= 0 {
		cfg.Job.Retry.Factor = 2.0
	}
	if cfg.Job.Retry.MaxRetries <= 0 {
		cfg.Job.Retry.MaxRetries = 3
	}

	// MQ defaults
	if cfg.MQ.Provider == MQProviderRabbitMQ {
		if cfg.MQ.RabbitMQ.DelayMode == "" {
			cfg.MQ.RabbitMQ.DelayMode = DelayModeStandard
		}
		// 提供统一的 Exchange 默认值，应用无需配置
		if cfg.MQ.RabbitMQ.Exchange == "" {
			cfg.MQ.RabbitMQ.Exchange = "taskbus.events"
		}
		if cfg.MQ.RabbitMQ.DelayedExchange == "" {
			cfg.MQ.RabbitMQ.DelayedExchange = "taskbus.events.delayed"
		}
		if cfg.MQ.RabbitMQ.Prefetch <= 0 {
			cfg.MQ.RabbitMQ.Prefetch = 64
		}
		if cfg.MQ.RabbitMQ.ConsumerConcurrency <= 0 {
			cfg.MQ.RabbitMQ.ConsumerConcurrency = 4
		}
	}
	if cfg.MQ.Provider == MQProviderRedis {
		if cfg.MQ.Redis.ConsumerConcurrency <= 0 {
			cfg.MQ.Redis.ConsumerConcurrency = 4
		}
	}

	// Cron defaults（默认分布式 + 基于 namespace 的锁键）
	if !cfg.Cron.Distributed {
		cfg.Cron.Distributed = true
	}
	if cfg.Cron.LeaderLockKey == "" {
		cfg.Cron.LeaderLockKey = fmt.Sprintf("taskbus:cron:%s", cfg.Namespace)
	}
	if cfg.Cron.LeaderTTL <= 0 {
		cfg.Cron.LeaderTTL = 30 * time.Second
	}

	// EventBus defaults
	if cfg.EventBus.Mode == "" {
		cfg.EventBus.Mode = EventBusModeIsolated
	}
	if cfg.EventBus.SubscriberConcurrency <= 0 {
		cfg.EventBus.SubscriberConcurrency = 1
	}
}
