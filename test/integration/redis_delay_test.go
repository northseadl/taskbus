package integration

import (
	"context"
	"os"
	"testing"
	"time"

	tq "github.com/northseadl/taskbus"
)

func TestRedis_DelayedMessage_Flow(t *testing.T) {
	addr := os.Getenv("TQ_REDIS_ADDR")
	if addr == "" { t.Skip("redis env not set; skipping test") }

	cfg := tq.Config{MQ: tq.MQConfig{Provider: tq.MQProviderRedis, Redis: tq.RedisConfig{Addr: addr}}}
	ctx := context.Background()
	client, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new: %v", err) }
	defer client.Close(ctx)

	topic := "it.redis.delay"
	recv := make(chan time.Time, 1)
	stop, err := client.MQ().Consume(ctx, topic, "g1", func(ctx context.Context, m tq.Message) error { recv <- time.Now(); return nil })
	if err != nil { t.Fatalf("consume: %v", err) }
	defer stop(ctx)

	start := time.Now()
	delay := 1200 * time.Millisecond
	if err := client.MQ().PublishDelay(ctx, tq.Message{Topic: topic, Body: []byte("hello")}, delay); err != nil { t.Fatalf("publishDelay: %v", err) }

	select {
	case got := <-recv:
		elapsed := got.Sub(start)
		if elapsed < delay { t.Fatalf("too early: %v < %v", elapsed, delay) }
		if elapsed > delay+3*time.Second { t.Fatalf("too late: %v > %v", elapsed, delay+3*time.Second) }
	case <-time.After(6 * time.Second):
		t.Fatalf("timeout waiting delayed message")
	}
}

