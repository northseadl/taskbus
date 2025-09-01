package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	tq "github.com/northseadl/taskbus"
)

func requireEnv(t *testing.T, k string) string {
	v := os.Getenv(k)
	if v == "" { t.Skipf("env %s not set; skipping integration", k) }
	return v
}

func TestRabbitMQ_EndToEnd(t *testing.T) {
	uri := requireEnv(t, "TQ_RABBITMQ_URI")
	ex := requireEnv(t, "TQ_RABBITMQ_EXCHANGE")
	dx := os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")
	cfg := tq.Config{MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex, DelayedExchange: dx}}}
	ctx := context.Background()
	client, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new: %v", err) }
	defer client.Close(ctx)

	topic := "it.tq.basic"
	done := make(chan struct{})
	stop, err := client.MQ().Consume(ctx, topic, "g1", func(ctx context.Context, m tq.Message) error { close(done); return nil })
	if err != nil { t.Fatalf("consume: %v", err) }
	defer stop(ctx)
	if err := client.MQ().Publish(ctx, tq.Message{Topic: topic, Body: []byte("hi")}); err != nil { t.Fatalf("pub: %v", err) }
	select { case <-done: case <-time.After(3*time.Second): t.Fatalf("timeout") }
}

type badJob struct{}
func (badJob) Name() string { return "it.alwaysfail" }
func (badJob) Execute(ctx context.Context, p []byte) error { return fmt.Errorf("fail") }

func TestJobs_Retry_DeadLetter(t *testing.T) {
	uri := requireEnv(t, "TQ_RABBITMQ_URI")
	ex := requireEnv(t, "TQ_RABBITMQ_EXCHANGE")
	dx := os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")
	cfg := tq.Config{
		MQ: tq.MQConfig{Provider: tq.MQProviderRabbitMQ, RabbitMQ: tq.RabbitMQConfig{URI: uri, Exchange: ex, DelayedExchange: dx}},
		Job: tq.JobConfig{Retry: tq.RetryConfig{Base: 100 * time.Millisecond, Factor: 2, MaxRetries: 2}, DeadLetterTopic: "it.dead.jobs"},
	}
	ctx := context.Background()
	client, err := tq.New(ctx, cfg)
	if err != nil { t.Fatalf("new: %v", err) }
	defer client.Close(ctx)

	// Start deadletter consumer first
	dl := make(chan struct{}, 1)
	ds, err := client.MQ().Consume(ctx, cfg.Job.DeadLetterTopic, "gdl", func(ctx context.Context, m tq.Message) error {
		select {
		case dl <- struct{}{}:
		default:
		}
		return nil
	})
	if err != nil { t.Fatalf("consume dl: %v", err) }
	defer ds(ctx)

	// Register a job that always fails and start workers
	bj := badJob{}
	client.Jobs().Register(bj)
	stop, err := client.Jobs().StartWorkers(ctx, map[string]int{bj.Name(): 1})
	if err != nil { t.Fatalf("workers: %v", err) }
	defer stop(ctx)

	// Give workers time to start
	time.Sleep(100 * time.Millisecond)

	// Enqueue the failing job
	if err := client.Jobs().Enqueue(ctx, bj.Name(), []byte("p")); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Wait for dead letter (should happen after 2 retries + original attempt)
	select {
	case <-dl:
		// Success - job went to dead letter queue
	case <-time.After(10 * time.Second):
		t.Fatalf("dead letter not received within timeout")
	}
}

