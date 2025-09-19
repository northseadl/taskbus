package main

import (
	"context"
	"fmt"
	"os"
	"time"

	taskbus "github.com/northseadl/taskbus"
)

func main() {
	ctx := context.Background()

	cfg := taskbus.Config{}
	if addr := os.Getenv("TQ_REDIS_ADDR"); addr != "" {
		cfg.MQ.Provider = taskbus.MQProviderRedis
		cfg.MQ.Redis.Addr = addr
		fmt.Println("[Bus] 使用 Redis MQ:", addr)
	} else if uri := os.Getenv("TQ_RABBITMQ_URI"); uri != "" {
		cfg.MQ.Provider = taskbus.MQProviderRabbitMQ
		cfg.MQ.RabbitMQ.URI = uri
		cfg.MQ.RabbitMQ.Exchange = os.Getenv("TQ_RABBITMQ_EXCHANGE")
		cfg.MQ.RabbitMQ.DelayedExchange = os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")
		fmt.Println("[Bus] 使用 RabbitMQ:", uri)
	} else {
		fmt.Println("[Bus] 未配置 MQ，使用 Noop（仅演示 API，不会实际路由）")
	}

	cli, err := taskbus.New(ctx, cfg)
	if err != nil { panic(err) }
	defer func() { _ = cli.Close(ctx) }()

	stop, err := cli.Bus().Subscribe("demo.topic", "g1", nil, func(ctx context.Context, e taskbus.Event) error {
		fmt.Printf("[Bus] 收到: type=%s subject=%s payload=%s\n", e.Type, e.Subject, string(e.Payload))
		return nil
	})
	if err != nil { panic(err) }
	defer func() { _ = stop(ctx) }()

	_ = cli.Bus().Publish(ctx, taskbus.Event{Topic: "demo.topic", Type: "greeting", Subject: "u1", Payload: []byte("hello")})
	fmt.Println("[Bus] 已发布事件，等待 1s...")
	time.Sleep(1 * time.Second)
	fmt.Println("[Bus] 结束")
}

