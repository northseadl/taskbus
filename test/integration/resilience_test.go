package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	tq "github.com/northseadl/taskbus"
)

// This test simulates resilience by closing the client and recreating it, ensuring no message loss.
func TestRabbitMQ_Resilience_Reconnect(t *testing.T) {
	uri := os.Getenv("TQ_RABBITMQ_URI")
	ex := os.Getenv("TQ_RABBITMQ_EXCHANGE")
	if uri == "" || ex == "" { t.Skip("rabbitmq env not set; skipping test") }

	cfg := tq.Config{MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex}}}
	ctx := context.Background()

	topic := "it.resilience"
	// 1) publish a few messages
	{
		c, err := tq.New(ctx, cfg)
		if err != nil { t.Fatalf("new: %v", err) }
		for i := 0; i < 5; i++ { _ = c.MQ().Publish(ctx, tq.Message{Topic: topic, Key: fmt.Sprintf("k%d", i), Body: []byte("x")}) }
		_ = c.Close(ctx)
	}

	// 2) create a new client and consume; messages should still be there
	c2, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new2: %v", err) }
	defer c2.Close(ctx)
	count := 0
	done := make(chan struct{})
	stop, err := c2.MQ().Consume(ctx, topic, "g1", func(ctx context.Context, m tq.Message) error {
		count++
		if count >= 5 { close(done) }
		return nil
	})
	if err != nil { t.Fatalf("consume: %v", err) }
	defer stop(ctx)

	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatalf("did not receive all messages after reconnect")
	}
}

