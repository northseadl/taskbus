package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	tq "github.com/northseadl/taskbus"
)

// This test simulates resilience by testing that consumers can reconnect and continue processing.
func TestRabbitMQ_Resilience_Reconnect(t *testing.T) {
	uri := os.Getenv("TQ_RABBITMQ_URI")
	ex := os.Getenv("TQ_RABBITMQ_EXCHANGE")
	if uri == "" || ex == "" { t.Skip("rabbitmq env not set; skipping test") }

	cfg := tq.Config{MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex, DelayedExchange: os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")}}}
	ctx := context.Background()

	topic := "it.resilience"

	// 1) Start consumer first to create the queue
	c1, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new c1: %v", err) }

	count := 0
	done := make(chan struct{})
	var doneOnce bool
	stop1, err := c1.MQ().Consume(ctx, topic, "g1", func(ctx context.Context, m tq.Message) error {
		count++
		if count >= 3 && !doneOnce {
			doneOnce = true
			close(done)
		}
		return nil
	})
	if err != nil { t.Fatalf("consume: %v", err) }

	// 2) Publish messages while consumer is active
	for i := 0; i < 3; i++ {
		_ = c1.MQ().Publish(ctx, tq.Message{Topic: topic, Key: fmt.Sprintf("k%d", i), Body: []byte("x")})
	}

	// 3) Wait for messages to be processed
	select {
	case <-done:
		// ok, messages processed
	case <-time.After(3 * time.Second):
		t.Fatalf("did not receive all messages: got %d, expected 3", count)
	}

	// 4) Stop consumer and close client (simulating disconnect)
	stop1(ctx)
	c1.Close(ctx)

	// 5) Create new client and verify it can connect and work
	c2, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new c2 after reconnect: %v", err) }
	defer c2.Close(ctx)

	// 6) Start consumer first, then publish
	received := make(chan struct{})
	var receivedOnce bool
	stop2, err := c2.MQ().Consume(ctx, topic, "g2", func(ctx context.Context, m tq.Message) error {
		if !receivedOnce {
			receivedOnce = true
			close(received)
		}
		return nil
	})
	if err != nil { t.Fatalf("consume after reconnect: %v", err) }
	defer stop2(ctx)

	// Give consumer time to start
	time.Sleep(100 * time.Millisecond)

	// 7) Test that new client can publish and consume
	if err := c2.MQ().Publish(ctx, tq.Message{Topic: topic, Key: "reconnect-test", Body: []byte("reconnect")}); err != nil {
		t.Fatalf("publish after reconnect: %v", err)
	}

	select {
	case <-received:
		// ok, reconnection successful
	case <-time.After(3 * time.Second):
		t.Fatalf("reconnected client failed to receive message")
	}
}

