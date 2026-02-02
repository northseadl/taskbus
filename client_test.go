package taskbus

import (
	"context"
	"testing"
	"time"
)

func TestNewClient_Defaults(t *testing.T) {
	ctx := context.Background()
	cfg := Config{}

	cli, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cli.Close(ctx)

	// 验证各子系统已初始化
	if cli.MQ() == nil {
		t.Error("MQ() should not be nil")
	}
	if cli.Jobs() == nil {
		t.Error("Jobs() should not be nil")
	}
	if cli.Cron() == nil {
		t.Error("Cron() should not be nil")
	}
	if cli.Bus() == nil {
		t.Error("Bus() should not be nil")
	}
	if cli.Streams() == nil {
		t.Error("Streams() should not be nil")
	}
}

func TestNewClient_WithNamespace(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Namespace: "test-service",
	}

	cli, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cli.Close(ctx)

	// Client 应该已创建成功
	if cli == nil {
		t.Error("New() returned nil client")
	}
}

func TestNewClient_InvalidNamespace(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		Namespace: "INVALID_NAMESPACE!", // 包含非法字符
	}

	cli, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cli.Close(ctx)

	// 非法命名空间应该回退到 "default"
	// Client 仍然应该创建成功
	if cli == nil {
		t.Error("New() returned nil client for invalid namespace")
	}
}

func TestApplyDefaultConfig(t *testing.T) {
	cfg := &Config{}
	applyDefaultConfig(cfg)

	// Job defaults
	if cfg.Job.DefaultGroup != "default" {
		t.Errorf("Job.DefaultGroup = %s, want default", cfg.Job.DefaultGroup)
	}
	if cfg.Job.Retry.Base != time.Second {
		t.Errorf("Job.Retry.Base = %v, want 1s", cfg.Job.Retry.Base)
	}
	if cfg.Job.Retry.Factor != 2.0 {
		t.Errorf("Job.Retry.Factor = %v, want 2.0", cfg.Job.Retry.Factor)
	}
	if cfg.Job.Retry.MaxRetries != 3 {
		t.Errorf("Job.Retry.MaxRetries = %d, want 3", cfg.Job.Retry.MaxRetries)
	}

	// Cron defaults
	if !cfg.Cron.Distributed {
		t.Error("Cron.Distributed should be true by default")
	}
	if cfg.Cron.LeaderTTL != 30*time.Second {
		t.Errorf("Cron.LeaderTTL = %v, want 30s", cfg.Cron.LeaderTTL)
	}

	// MQ retry defaults
	if cfg.MQ.Retry.Base != cfg.Job.Retry.Base {
		t.Errorf("MQ.Retry.Base = %v, want %v", cfg.MQ.Retry.Base, cfg.Job.Retry.Base)
	}
	if cfg.MQ.Retry.Factor != cfg.Job.Retry.Factor {
		t.Errorf("MQ.Retry.Factor = %v, want %v", cfg.MQ.Retry.Factor, cfg.Job.Retry.Factor)
	}
	if cfg.MQ.Retry.MaxRetries != cfg.Job.Retry.MaxRetries {
		t.Errorf("MQ.Retry.MaxRetries = %d, want %d", cfg.MQ.Retry.MaxRetries, cfg.Job.Retry.MaxRetries)
	}

	// EventBus defaults
	if cfg.EventBus.Mode != EventBusModeIsolated {
		t.Errorf("EventBus.Mode = %s, want isolated", cfg.EventBus.Mode)
	}
	if cfg.EventBus.SubscriberConcurrency != 1 {
		t.Errorf("EventBus.SubscriberConcurrency = %d, want 1", cfg.EventBus.SubscriberConcurrency)
	}
}

func TestApplyDefaultConfig_RabbitMQ(t *testing.T) {
	cfg := &Config{
		MQ: MQConfig{
			Provider: MQProviderRabbitMQ,
		},
	}
	applyDefaultConfig(cfg)

	if cfg.MQ.RabbitMQ.DelayMode != DelayModeStandard {
		t.Errorf("RabbitMQ.DelayMode = %s, want standard", cfg.MQ.RabbitMQ.DelayMode)
	}
	if cfg.MQ.RabbitMQ.Exchange != "taskbus.events" {
		t.Errorf("RabbitMQ.Exchange = %s, want taskbus.events", cfg.MQ.RabbitMQ.Exchange)
	}
	if cfg.MQ.RabbitMQ.DelayedExchange != "taskbus.events.delayed" {
		t.Errorf("RabbitMQ.DelayedExchange = %s, want taskbus.events.delayed", cfg.MQ.RabbitMQ.DelayedExchange)
	}
	if cfg.MQ.RabbitMQ.Prefetch != 64 {
		t.Errorf("RabbitMQ.Prefetch = %d, want 64", cfg.MQ.RabbitMQ.Prefetch)
	}
	if cfg.MQ.RabbitMQ.ConsumerConcurrency != 4 {
		t.Errorf("RabbitMQ.ConsumerConcurrency = %d, want 4", cfg.MQ.RabbitMQ.ConsumerConcurrency)
	}
}

func TestApplyDefaultConfig_Redis(t *testing.T) {
	cfg := &Config{
		MQ: MQConfig{
			Provider: MQProviderRedis,
		},
	}
	applyDefaultConfig(cfg)

	if cfg.MQ.Redis.ConsumerConcurrency != 4 {
		t.Errorf("Redis.ConsumerConcurrency = %d, want 4", cfg.MQ.Redis.ConsumerConcurrency)
	}
}

func TestIsValidNamespace(t *testing.T) {
	tests := []struct {
		namespace string
		valid     bool
	}{
		{"default", true},
		{"my-service", true},
		{"myservice123", true},
		{"my-service-v2", true},
		{"a", true},
		{"ab", true},
		{"", false},
		{"My-Service", false}, // 大写
		{"my_service", false}, // 下划线
		{"-myservice", false}, // 以连字符开头
		{"myservice-", false}, // 以连字符结尾
		{"my--service", true}, // 连续连字符（允许）
		{"my.service", false}, // 点号
		{"my service", false}, // 空格
	}

	for _, tt := range tests {
		t.Run(tt.namespace, func(t *testing.T) {
			got := isValidNamespace(tt.namespace)
			if got != tt.valid {
				t.Errorf("isValidNamespace(%q) = %v, want %v", tt.namespace, got, tt.valid)
			}
		})
	}
}

func TestWithLogger(t *testing.T) {
	ctx := context.Background()

	// 自定义 Logger
	customLogger := &testLogger{}

	cfg := Config{}
	cli, err := New(ctx, cfg, WithLogger(customLogger))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cli.Close(ctx)

	// Client 应该创建成功
	if cli == nil {
		t.Error("New() returned nil client")
	}
}

type testLogger struct {
	infoCalled  bool
	errorCalled bool
}

func (l *testLogger) Info(ctx context.Context, msg string, kv ...interface{}) {
	l.infoCalled = true
}

func (l *testLogger) Error(ctx context.Context, msg string, kv ...interface{}) {
	l.errorCalled = true
}

func TestNoopImplementations(t *testing.T) {
	ctx := context.Background()

	t.Run("noopMQ", func(t *testing.T) {
		mq := newNoopMQ()
		if err := mq.Publish(ctx, Message{}); err != nil {
			t.Errorf("noopMQ.Publish() error = %v", err)
		}
		if err := mq.PublishDelay(ctx, Message{}, time.Second); err != nil {
			t.Errorf("noopMQ.PublishDelay() error = %v", err)
		}
		stop, err := mq.Consume(ctx, "topic", "group", func(ctx context.Context, msg Message) error { return nil })
		if err != nil {
			t.Errorf("noopMQ.Consume() error = %v", err)
		}
		if stop == nil {
			t.Error("noopMQ.Consume() stop function is nil")
		}
		if err := mq.Close(ctx); err != nil {
			t.Errorf("noopMQ.Close() error = %v", err)
		}
	})

	t.Run("noopJobs", func(t *testing.T) {
		jobs := newNoopJobs()
		jobs.Register(nil)
		if err := jobs.Enqueue(ctx, "job", nil); err != nil {
			t.Errorf("noopJobs.Enqueue() error = %v", err)
		}
		stop, err := jobs.StartWorkers(ctx, nil)
		if err != nil {
			t.Errorf("noopJobs.StartWorkers() error = %v", err)
		}
		if stop == nil {
			t.Error("noopJobs.StartWorkers() stop function is nil")
		}
	})

	t.Run("noopBus", func(t *testing.T) {
		bus := newNoopBus()
		if err := bus.Publish(ctx, Event{}); err != nil {
			t.Errorf("noopBus.Publish() error = %v", err)
		}
		stop, err := bus.Subscribe("topic", "group", nil, func(ctx context.Context, e Event) error { return nil })
		if err != nil {
			t.Errorf("noopBus.Subscribe() error = %v", err)
		}
		if stop == nil {
			t.Error("noopBus.Subscribe() stop function is nil")
		}
	})

	t.Run("noopCron", func(t *testing.T) {
		cron := newNoopCron()
		id, err := cron.Add("* * * * * *", "test", func(ctx context.Context) error { return nil })
		if err != nil {
			t.Errorf("noopCron.Add() error = %v", err)
		}
		if id != "" {
			t.Errorf("noopCron.Add() id = %s, want empty", id)
		}
		if err := cron.Remove("test"); err != nil {
			t.Errorf("noopCron.Remove() error = %v", err)
		}
		if err := cron.Start(ctx); err != nil {
			t.Errorf("noopCron.Start() error = %v", err)
		}
		if err := cron.Stop(ctx); err != nil {
			t.Errorf("noopCron.Stop() error = %v", err)
		}
	})
}
