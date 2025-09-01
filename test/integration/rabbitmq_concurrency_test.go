package integration

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	tq "github.com/northseadl/taskbus"
)

func TestRabbitMQ_Concurrency(t *testing.T) {
	uri := os.Getenv("TQ_RABBITMQ_URI")
	ex := os.Getenv("TQ_RABBITMQ_EXCHANGE")
	if uri == "" || ex == "" { t.Skip("rabbitmq env not set; skipping test") }

	cfg := tq.Config{MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex, Prefetch: 64, ConsumerConcurrency: 8}}}
	ctx := context.Background()
	c, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new: %v", err) }
	defer c.Close(ctx)

	topic := "it.concurrent"
	var processed int64
	stop, err := c.MQ().Consume(ctx, topic, "g1", func(ctx context.Context, m tq.Message) error { atomic.AddInt64(&processed, 1); time.Sleep(50 * time.Millisecond); return nil })
	if err != nil { t.Fatalf("consume: %v", err) }
	defer stop(ctx)

	// publish backlog of messages
	n := 100
	for i := 0; i < n; i++ { _ = c.MQ().Publish(ctx, tq.Message{Topic: topic, Key: time.Now().Format(time.RFC3339Nano), Body: []byte("x")}) }

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&processed) >= int64(n) { return }
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("not all messages processed in time: %d/%d", processed, n)
}

