package integration

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	tq "github.com/northseadl/taskbus"
)

func TestCron_Distributed(t *testing.T) {
	uri := os.Getenv("TQ_RABBITMQ_URI")
	ex := os.Getenv("TQ_RABBITMQ_EXCHANGE")
	if uri == "" || ex == "" { t.Skip("rabbitmq env not set; skipping test") }

	cfg := tq.Config{
		MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex, DelayedExchange: os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")}},
		Cron: tq.CronConfig{Distributed: true, ExecutorGroup: "it-cron"},
	}
	ctx := context.Background()
	c1, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new c1: %v", err) }
	defer c1.Close(ctx)
	c2, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new c2: %v", err) }
	defer c2.Close(ctx)

	var n int64
	id, err := c1.Cron().Add("*/1 * * * * *", "it.task", func(ctx context.Context) error { atomic.AddInt64(&n, 1); return nil })
	if err != nil { t.Fatalf("cron add: %v", err) }
	defer c1.Cron().Remove(id)
	_ = c1.Cron().Start(ctx)
	defer c1.Cron().Stop(ctx)
	_ = c2.Cron().Start(ctx)
	defer c2.Cron().Stop(ctx)

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&n) >= 1 { return }
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("cron not executed in time")
}

