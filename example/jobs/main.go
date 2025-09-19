package main

import (
	"context"
	"fmt"
	"os"
	"time"

	taskbus "github.com/northseadl/taskbus"
)

// echoJob 一个简单任务: 打印载荷
type echoJob struct{}

func (e echoJob) Name() string { return "example.echo" }
func (e echoJob) Execute(ctx context.Context, payload []byte) error {
	fmt.Println("[Jobs] 执行:", string(payload))
	return nil
}

func main() {
	ctx := context.Background()

	// 基础配置：默认使用 Noop MQ；若设置了环境变量则启用 Redis/RabbitMQ
	cfg := taskbus.Config{
		Job: taskbus.JobConfig{Retry: taskbus.RetryConfig{Base: time.Second, Factor: 2, MaxRetries: 3}},
	}
	if addr := os.Getenv("TQ_REDIS_ADDR"); addr != "" {
		cfg.MQ.Provider = taskbus.MQProviderRedis
		cfg.MQ.Redis.Addr = addr
		fmt.Println("[Jobs] 使用 Redis MQ:", addr)
	} else if uri := os.Getenv("TQ_RABBITMQ_URI"); uri != "" {
		cfg.MQ.Provider = taskbus.MQProviderRabbitMQ
		cfg.MQ.RabbitMQ.URI = uri
		cfg.MQ.RabbitMQ.Exchange = os.Getenv("TQ_RABBITMQ_EXCHANGE")
		cfg.MQ.RabbitMQ.DelayedExchange = os.Getenv("TQ_RABBITMQ_DELAYED_EXCHANGE")
		fmt.Println("[Jobs] 使用 RabbitMQ:", uri)
	} else {
		fmt.Println("[Jobs] 未配置 MQ，使用 Noop（仅演示 API，消息不会实际路由）")
	}

	cli, err := taskbus.New(ctx, cfg)
	if err != nil { panic(err) }
	defer func() { _ = cli.Close(ctx) }()

	// 注册 Job 并启动 Worker
	cli.Jobs().Register(echoJob{})
	stop, err := cli.Jobs().StartWorkers(ctx, map[string]int{"default": 1})
	if err != nil { panic(err) }
	defer func() { _ = stop(ctx) }()

	// 入队 3 条任务
	_ = cli.Jobs().Enqueue(ctx, "example.echo", []byte("hello"))
	_ = cli.Jobs().Enqueue(ctx, "example.echo", []byte("world"), taskbus.WithDelay(500*time.Millisecond))
	_ = cli.Jobs().Enqueue(ctx, "example.echo", []byte("taskbus"), taskbus.WithKey("demo-1"))

	fmt.Println("[Jobs] 已入队示例任务，等待 2s 以便处理...")
	time.Sleep(2 * time.Second)
	fmt.Println("[Jobs] 结束")
}

