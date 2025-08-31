package taskbus

import (
	"context"

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
}

// New 创建 Client 实例。
func New(ctx context.Context, cfg Config, opts ...Option) (Client, error) {
	c := &client{
		cfg:    cfg,
		logger: defaultLogger{},
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
		mq, err := newRabbitMQAdapterWithMode(cfg.MQ.RabbitMQ, mode, c.logger)
		if err != nil {
			return nil, err
		}
		c.mq = mq
	case MQProviderRedis:
		mq, err := newRedisAdapter(cfg.MQ.Redis, c.logger)
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
}

func (c *client) Start(ctx context.Context) error { return nil }
func (c *client) Close(ctx context.Context) error {
	if c.mq != nil {
		_ = c.mq.Close(ctx)
	}
	return nil
}
func (c *client) MQ() MQ        { return c.mq }
func (c *client) Jobs() Jobs    { return c.jobs }
func (c *client) Cron() Cron    { return c.cron }
func (c *client) Bus() EventBus { return c.bus }

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
