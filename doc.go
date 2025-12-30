// Package taskbus 提供统一的任务队列、定时调度、发布订阅与事件总线能力。
//
// # 核心特性
//
//   - MQ 适配器：支持 RabbitMQ 和 Redis Streams，通过配置切换
//   - Jobs：任务队列，支持延时、重试、指数退避和死信处理
//   - EventBus：事件发布/订阅，支持类型过滤和命名空间隔离
//   - Cron：定时任务，支持本地和分布式模式（基于 MQ Leader 选举）
//   - 幂等中间件：内置 Redis 幂等检查，防止重复处理
//
// # 快速开始
//
//	package main
//
//	import (
//	    "context"
//	    "github.com/northseadl/taskbus"
//	)
//
//	func main() {
//	    cfg := taskbus.Config{
//	        Namespace: "my-service",
//	        MQ: taskbus.MQConfig{
//	            Provider: taskbus.MQProviderRabbitMQ,
//	            RabbitMQ: taskbus.RabbitMQConfig{
//	                URI:      "amqp://localhost:5672",
//	                Exchange: "app.events",
//	            },
//	        },
//	    }
//
//	    ctx := context.Background()
//	    cli, _ := taskbus.New(ctx, cfg)
//	    defer cli.Close(ctx)
//
//	    // 发布事件
//	    cli.Bus().Publish(ctx, taskbus.Event{
//	        Topic:   "user.created",
//	        Type:    "UserCreated",
//	        Subject: "user-123",
//	        Payload: []byte(`{"name": "Alice"}`),
//	    })
//	}
//
// # 架构设计
//
// TaskBus 采用可插拔的适配器模式：
//
//	┌─────────────────────────────────────────┐
//	│              Client (统一入口)            │
//	├─────────┬─────────┬─────────┬───────────┤
//	│  Jobs   │ EventBus│  Cron   │  Streams  │
//	├─────────┴─────────┴─────────┴───────────┤
//	│              MQ (抽象层)                  │
//	├─────────────────┬───────────────────────┤
//	│  RabbitMQ 适配器 │   Redis Streams 适配器 │
//	└─────────────────┴───────────────────────┘
//
// # 命名空间隔离
//
// 通过 Config.Namespace 配置，TaskBus 自动为 topic、消费组和锁键添加前缀，
// 支持多服务在同一 MQ 集群上安全共存：
//
//	Jobs topic:    taskbus.{namespace}.job.{jobName}
//	Event topic:   taskbus.{namespace}.event.{topic}
//	Cron topic:    taskbus.{namespace}.cron.{name}
//
// # 中间件系统
//
// TaskBus 提供四层中间件用于扩展：
//
//   - Middleware：MQ 消息处理中间件
//   - JobMiddleware：Job 执行中间件
//   - EventMiddleware：EventBus 事件中间件
//   - CronMiddleware：Cron 任务中间件
//
// 内置中间件包括幂等检查（IdempotencyMiddleware）。
package taskbus
