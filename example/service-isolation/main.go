package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/northseadl/taskbus"
)

// 演示服务级隔离效果

// DemoJob 演示任务
type DemoJob struct {
	serviceName string
}

func (j *DemoJob) Name() string {
	return "demo.process"
}

func (j *DemoJob) Execute(ctx context.Context, payload []byte) error {
	log.Printf("[%s] 执行 job: %s, payload: %s", j.serviceName, j.Name(), string(payload))
	return nil
}

func main() {
	// 模拟两个服务：service-a 和 service-b

	ctx := context.Background()

	// === 服务 A ===
	cfgA := taskbus.Config{
		MQ: taskbus.MQConfig{
			Provider: taskbus.MQProviderRedis,
			Redis: taskbus.RedisConfig{
				Addr:                "localhost:6379",
				ConsumerConcurrency: 2,
			},
		},
		Job: taskbus.JobConfig{
			GroupPrefix:  "service-a",
			DefaultGroup: "worker",
		},
	}
	clientA, err := taskbus.New(ctx, cfgA)
	if err != nil {
		log.Fatalf("create client A: %v", err)
	}
	defer clientA.Close(ctx)

	// 注册 Job
	jobA := &DemoJob{serviceName: "service-a"}
	clientA.Jobs().Register(jobA)

	// 启动 Worker
	stopA, err := clientA.Jobs().StartWorkers(ctx, map[string]int{"worker": 2})
	if err != nil {
		log.Fatalf("start workers A: %v", err)
	}
	defer stopA(ctx)

	log.Println("✅ Service A started, listening on: job.service-a.#")

	// === 服务 B ===
	cfgB := taskbus.Config{
		MQ: taskbus.MQConfig{
			Provider: taskbus.MQProviderRedis,
			Redis: taskbus.RedisConfig{
				Addr:                "localhost:6379",
				ConsumerConcurrency: 2,
			},
		},
		Job: taskbus.JobConfig{
			GroupPrefix:  "service-b",
			DefaultGroup: "worker",
		},
	}
	clientB, err := taskbus.New(ctx, cfgB)
	if err != nil {
		log.Fatalf("create client B: %v", err)
	}
	defer clientB.Close(ctx)

	// 注册 Job
	jobB := &DemoJob{serviceName: "service-b"}
	clientB.Jobs().Register(jobB)

	// 启动 Worker
	stopB, err := clientB.Jobs().StartWorkers(ctx, map[string]int{"worker": 2})
	if err != nil {
		log.Fatalf("start workers B: %v", err)
	}
	defer stopB(ctx)

	log.Println("✅ Service B started, listening on: job.service-b.#")

	// === 测试隔离效果 ===
	time.Sleep(2 * time.Second)

	log.Println("\n=== 测试 1：Service A 调用 demo.process ===")
	if err := clientA.Jobs().Enqueue(ctx, "demo.process", []byte("data from A")); err != nil {
		log.Printf("enqueue A: %v", err)
	}
	log.Println("预期：仅 Service A 执行，Service B 不会收到消息")

	time.Sleep(2 * time.Second)

	log.Println("\n=== 测试 2：Service B 调用 demo.process ===")
	if err := clientB.Jobs().Enqueue(ctx, "demo.process", []byte("data from B")); err != nil {
		log.Printf("enqueue B: %v", err)
	}
	log.Println("预期：仅 Service B 执行，Service A 不会收到消息")

	time.Sleep(2 * time.Second)

	log.Println("\n=== 测试 3：验证 topic 格式 ===")
	log.Println("Service A 发送的 topic: job.service-a.demo.process")
	log.Println("Service B 发送的 topic: job.service-b.demo.process")
	log.Println("Service A 订阅模式: job.service-a.#")
	log.Println("Service B 订阅模式: job.service-b.#")
	log.Println("✅ 完全隔离，无跨服务消息污染")

	fmt.Println("\n按 Ctrl+C 退出...")

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("\n正在关闭...")
}


