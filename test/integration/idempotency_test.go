package integration

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	tq "github.com/northseadl/taskbus"
)

type idemJob struct{ cnt *int64 }
func (idemJob) Name() string { return "it.idem.job" }
func (j idemJob) Execute(ctx context.Context, p []byte) error { atomic.AddInt64(j.cnt, 1); return nil }

func TestIdempotency_Jobs(t *testing.T) {
	uri := os.Getenv("TQ_RABBITMQ_URI")
	ex := os.Getenv("TQ_RABBITMQ_EXCHANGE")
	ra := os.Getenv("TQ_REDIS_ADDR")
	if uri == "" || ex == "" || ra == "" { t.Skip("env not set; skipping test") }

	rdb := redis.NewClient(&redis.Options{Addr: ra})
	defer rdb.Close()

	cfg := tq.Config{
		MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex, DelayedExchange: os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")}},
		Idempotency: tq.IdempotencyConfig{KV: tq.RedisKV{R: rdb}, Prefix: "tq:test:idem", TTL: 10 * time.Second},
	}
	ctx := context.Background()
	c, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new: %v", err) }
	defer c.Close(ctx)


	var n int64
	c.Jobs().Register(idemJob{cnt: &n})
	stop, err := c.Jobs().StartWorkers(ctx, map[string]int{"g1":1})
	if err != nil { t.Fatalf("workers: %v", err) }
	defer stop(ctx)

	// Give workers time to start
	time.Sleep(100 * time.Millisecond)

	key := "k1"
	for i := 0; i < 5; i++ {
		if err := c.Jobs().Enqueue(ctx, "it.idem.job", []byte("p"), tq.WithKey(key)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	time.Sleep(3 * time.Second)
	got := atomic.LoadInt64(&n)
	if got != 1 { t.Fatalf("expected 1 execution, got %d", got) }
}

func TestIdempotency_EventBus(t *testing.T) {
	uri := os.Getenv("TQ_RABBITMQ_URI")
	ex := os.Getenv("TQ_RABBITMQ_EXCHANGE")
	ra := os.Getenv("TQ_REDIS_ADDR")
	if uri == "" || ex == "" || ra == "" { t.Skip("env not set; skipping test") }

	rdb := redis.NewClient(&redis.Options{Addr: ra})
	defer rdb.Close()

	cfg := tq.Config{
		MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex, DelayedExchange: os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")}},
		Idempotency: tq.IdempotencyConfig{KV: tq.RedisKV{R: rdb}, Prefix: "tq:test:idem:bus", TTL: 10 * time.Second},
	}
	ctx := context.Background()
	c, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new: %v", err) }
	defer c.Close(ctx)

	topic := "it.idem.bus"
	var rcv int64
	stop, err := c.Bus().Subscribe(topic, "g1", nil, func(ctx context.Context, e tq.Event) error { atomic.AddInt64(&rcv, 1); return nil })
	if err != nil { t.Fatalf("sub: %v", err) }
	defer stop(ctx)
	e := tq.Event{Topic: topic, Type: "T", Subject: "s1", Payload: []byte("p")}
	for i := 0; i < 5; i++ { _ = c.Bus().Publish(ctx, e) }
	time.Sleep(2 * time.Second)
	if atomic.LoadInt64(&rcv) != 1 { t.Fatalf("expected 1 event, got %d", rcv) }
}

