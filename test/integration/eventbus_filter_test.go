package integration

import (
	"context"
	"os"
	"testing"
	"time"

	tq "github.com/northseadl/taskbus"
)

func TestEventBus_FilterByType(t *testing.T) {
	uri := os.Getenv("TQ_RABBITMQ_URI")
	ex := os.Getenv("TQ_RABBITMQ_EXCHANGE")
	if uri == "" || ex == "" { t.Skip("rabbitmq env not set; skipping test") }

	cfg := tq.Config{MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex, DelayedExchange: os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")}}}
	ctx := context.Background()
	client, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new: %v", err) }
	defer client.Close(ctx)

	topic := "it.event.filter"
	recv := make(chan tq.Event, 10)
	stop, err := client.Bus().Subscribe(topic, "g1", tq.FilterByType("OrderCreated"), func(ctx context.Context, e tq.Event) error { recv <- e; return nil })
	if err != nil { t.Fatalf("subscribe: %v", err) }
	defer stop(ctx)

	// publish mixed types
	pub := func(typ string, subj string){ _ = client.Bus().Publish(ctx, tq.Event{Topic: topic, Type: typ, Subject: subj, Payload: []byte("{}")}) }
	pub("OrderCreated", "o1")
	pub("OrderUpdated", "o1")
	pub("OrderCreated", "o2")

	deadline := time.Now().Add(3 * time.Second)
	var got []tq.Event
	for time.Now().Before(deadline) && len(got) < 2 {
		select {
		case e := <-recv:
			got = append(got, e)
		case <-time.After(100 * time.Millisecond):
		}
	}
	if len(got) != 2 { t.Fatalf("expected 2 filtered events, got %d", len(got)) }
	for _, e := range got { if e.Type != "OrderCreated" { t.Fatalf("unexpected type: %s", e.Type) } }
}

