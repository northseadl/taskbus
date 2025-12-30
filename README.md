# TaskBus

[![Go Reference](https://pkg.go.dev/badge/github.com/northseadl/taskbus.svg)](https://pkg.go.dev/github.com/northseadl/taskbus)
[![Go Report Card](https://goreportcard.com/badge/github.com/northseadl/taskbus)](https://goreportcard.com/report/github.com/northseadl/taskbus)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A high-performance, production-ready Go task queue and event bus library. Supports RabbitMQ/Redis, middleware chains, delayed messages, idempotency, retry/dead-letter, and local/distributed Cron scheduling.

[English](#english) | [中文](#中文)

---

<a name="english"></a>
## English

### Features

- **MQ Adapters**: RabbitMQ, Redis Streams (instant/delayed messages)
- **Jobs**: Task queue with retry, exponential backoff, dead-letter handling
- **EventBus**: Pub/Sub with type filtering and namespace isolation
- **Cron**: Local/distributed scheduling (MQ-based leader election)
- **Idempotency**: Built-in Redis idempotency middleware
- **Production Ready**: Logger interface, error handling, connection recovery, graceful shutdown

### Installation

```bash
go get github.com/northseadl/taskbus
```

### Quick Start

#### Basic Usage

```go
package main

import (
    "context"
    "log"

    "github.com/northseadl/taskbus"
)

func main() {
    cfg := taskbus.Config{
        Namespace: "my-service",
        MQ: taskbus.MQConfig{
            Provider: taskbus.MQProviderRabbitMQ,
            RabbitMQ: taskbus.RabbitMQConfig{
                URI:      "amqp://localhost:5672",
                Exchange: "app.events",
            },
        },
    }

    ctx := context.Background()
    cli, err := taskbus.New(ctx, cfg)
    if err != nil { log.Fatal(err) }
    defer cli.Close(ctx)

    // Publish event
    _ = cli.Bus().Publish(ctx, taskbus.Event{
        Topic:   "user.created",
        Type:    "UserCreated",
        Subject: "uid-1",
        Payload: []byte(`{"name": "Alice"}`),
    })
}
```

#### Jobs - Task Queue

```go
// Define a job
type SendEmailJob struct{}

func (SendEmailJob) Name() string { return "email.send" }
func (SendEmailJob) Execute(ctx context.Context, payload []byte) error {
    log.Println("Sending email:", string(payload))
    return nil
}

// Register and start workers
cli.Jobs().Register(SendEmailJob{})
stop, _ := cli.Jobs().StartWorkers(ctx, map[string]int{"default": 4})
defer stop(ctx)

// Enqueue tasks
cli.Jobs().Enqueue(ctx, "email.send", []byte(`{"to": "user@example.com"}`))

// Delayed task
cli.Jobs().Enqueue(ctx, "email.send", payload, taskbus.WithDelay(5*time.Minute))

// With idempotency key
cli.Jobs().Enqueue(ctx, "email.send", payload, taskbus.WithKey("order-123"))
```

#### Cron - Scheduled Tasks

```go
// Every minute
cli.Cron().Add("0 * * * * *", "cleanup", func(ctx context.Context) error {
    return doCleanup()
})

// Daily at 2 AM
cli.Cron().Add("0 0 2 * * *", "daily-report", generateDailyReport)

cli.Cron().Start(ctx)
defer cli.Cron().Stop(ctx)
```

#### EventBus - Pub/Sub

```go
// Publish event
cli.Bus().Publish(ctx, taskbus.Event{
    Topic:   "order.paid",
    Type:    "OrderPaid",
    Subject: "order-456",
    Payload: orderJSON,
})

// Subscribe with type filter
stop, _ := cli.Bus().Subscribe("order.#", "inventory-service",
    taskbus.FilterByType("OrderPaid"),
    func(ctx context.Context, e taskbus.Event) error {
        log.Println("Order paid:", e.Subject)
        return nil
    },
)
defer stop(ctx)
```

### Architecture

```
┌─────────────────────────────────────────┐
│              Client (Unified Entry)      │
├─────────┬─────────┬─────────┬───────────┤
│  Jobs   │ EventBus│  Cron   │  Streams  │
├─────────┴─────────┴─────────┴───────────┤
│              MQ (Abstraction Layer)      │
├─────────────────┬───────────────────────┤
│ RabbitMQ Adapter│  Redis Streams Adapter│
└─────────────────┴───────────────────────┘
```

### Namespace Isolation

Topics are automatically prefixed with namespace for multi-service isolation:

| Component | Topic Format |
|-----------|-------------|
| Jobs | `taskbus.{namespace}.job.{jobName}` |
| EventBus | `taskbus.{namespace}.event.{topic}` |
| Cron | `taskbus.{namespace}.cron.{name}` |

### Middleware

TaskBus provides four middleware layers:

```go
// MQ Middleware
type Middleware func(next Handler) Handler

// Job Middleware
type JobMiddleware func(next JobHandler) JobHandler

// Event Middleware
type EventMiddleware func(next func(context.Context, Event) error) func(context.Context, Event) error

// Cron Middleware
type CronMiddleware func(next func(context.Context) error) func(context.Context) error
```

#### Custom Middleware Example

```go
func LoggingMiddleware(next taskbus.Handler) taskbus.Handler {
    return func(ctx context.Context, msg taskbus.Message) error {
        log.Printf("Processing: %s", msg.Topic)
        start := time.Now()
        err := next(ctx, msg)
        log.Printf("Completed in %v, error: %v", time.Since(start), err)
        return err
    }
}
```

### Configuration

```go
cfg := taskbus.Config{
    Namespace: "my-service",
    
    MQ: taskbus.MQConfig{
        Provider: taskbus.MQProviderRabbitMQ,
        RabbitMQ: taskbus.RabbitMQConfig{
            URI:                 "amqp://localhost:5672",
            Exchange:            "app.events",
            DelayedExchange:     "app.events.delayed",
            Prefetch:            64,
            ConsumerConcurrency: 4,
            DelayMode:           taskbus.DelayModeStandard,
        },
    },
    
    Job: taskbus.JobConfig{
        DefaultGroup: "default",
        Retry: taskbus.RetryConfig{
            Base:       time.Second,
            Factor:     2.0,
            MaxRetries: 3,
        },
    },
    
    Cron: taskbus.CronConfig{
        Distributed: true,
        Timezone:    "Asia/Shanghai",
        LeaderTTL:   30 * time.Second,
    },
    
    Idempotency: taskbus.IdempotencyConfig{
        RedisAddr: "localhost:6379",
        Prefix:    "idem",
        TTL:       24 * time.Hour,
    },
}
```

### Examples

See the [example](./example) directory for complete examples:

- `example/jobs` - Jobs task queue
- `example/cron` - Cron scheduled tasks
- `example/eventbus` - EventBus pub/sub
- `example/service-isolation` - Multi-service namespace isolation

### Testing

```bash
go test -v ./...
go test -cover ./...
```

---

<a name="中文"></a>
## 中文

### 特性

- **MQ 适配**：RabbitMQ、Redis Streams（即时/延迟消息）
- **Jobs**：任务队列，支持重试、指数回退、死信处理
- **EventBus**：发布/订阅、类型过滤、命名空间隔离
- **Cron**：本地/分布式执行（基于 MQ Leader 选举）
- **幂等**：内置 Redis 幂等中间件
- **生产就绪**：日志接口、错误处理、连接恢复、优雅停止

### 安装

```bash
go get github.com/northseadl/taskbus
```

### 快速开始

#### 基础用法

```go
package main

import (
    "context"
    "log"

    "github.com/northseadl/taskbus"
)

func main() {
    cfg := taskbus.Config{
        Namespace: "my-service",
        MQ: taskbus.MQConfig{
            Provider: taskbus.MQProviderRabbitMQ,
            RabbitMQ: taskbus.RabbitMQConfig{
                URI:      "amqp://localhost:5672",
                Exchange: "app.events",
            },
        },
    }

    ctx := context.Background()
    cli, err := taskbus.New(ctx, cfg)
    if err != nil { log.Fatal(err) }
    defer cli.Close(ctx)

    // 发布事件
    _ = cli.Bus().Publish(ctx, taskbus.Event{
        Topic:   "user.created",
        Type:    "UserCreated",
        Subject: "uid-1",
        Payload: []byte(`{"name": "Alice"}`),
    })
}
```

#### Jobs 任务队列

```go
// 定义任务
type SendEmailJob struct{}

func (SendEmailJob) Name() string { return "email.send" }
func (SendEmailJob) Execute(ctx context.Context, payload []byte) error {
    log.Println("发送邮件:", string(payload))
    return nil
}

// 注册并启动
cli.Jobs().Register(SendEmailJob{})
stop, _ := cli.Jobs().StartWorkers(ctx, map[string]int{"default": 4})
defer stop(ctx)

// 入队任务
cli.Jobs().Enqueue(ctx, "email.send", []byte(`{"to": "user@example.com"}`))

// 延迟任务
cli.Jobs().Enqueue(ctx, "email.send", payload, taskbus.WithDelay(5*time.Minute))

// 带幂等键（防重复）
cli.Jobs().Enqueue(ctx, "email.send", payload, taskbus.WithKey("order-123"))
```

#### Cron 定时任务

```go
// 每分钟执行
cli.Cron().Add("0 * * * * *", "cleanup", func(ctx context.Context) error {
    return doCleanup()
})

// 每天凌晨 2 点执行
cli.Cron().Add("0 0 2 * * *", "daily-report", generateDailyReport)

cli.Cron().Start(ctx)
defer cli.Cron().Stop(ctx)
```

#### EventBus 事件总线

```go
// 发布事件
cli.Bus().Publish(ctx, taskbus.Event{
    Topic:   "order.paid",
    Type:    "OrderPaid",
    Subject: "order-456",
    Payload: orderJSON,
})

// 订阅事件（带类型过滤）
stop, _ := cli.Bus().Subscribe("order.#", "inventory-service",
    taskbus.FilterByType("OrderPaid"),
    func(ctx context.Context, e taskbus.Event) error {
        log.Println("订单已支付:", e.Subject)
        return nil
    },
)
defer stop(ctx)
```

### 架构设计

```
┌─────────────────────────────────────────┐
│              Client (统一入口)            │
├─────────┬─────────┬─────────┬───────────┤
│  Jobs   │ EventBus│  Cron   │  Streams  │
├─────────┴─────────┴─────────┴───────────┤
│              MQ (抽象层)                  │
├─────────────────┬───────────────────────┤
│  RabbitMQ 适配器 │   Redis Streams 适配器 │
└─────────────────┴───────────────────────┘
```

### 命名空间隔离

通过 `Config.Namespace` 配置，TaskBus 自动为 topic 添加前缀，实现多服务隔离：

| 组件 | Topic 格式 |
|------|-----------|
| Jobs | `taskbus.{namespace}.job.{jobName}` |
| EventBus | `taskbus.{namespace}.event.{topic}` |
| Cron | `taskbus.{namespace}.cron.{name}` |

### 中间件

TaskBus 提供四层中间件用于扩展：

```go
// MQ 中间件
type Middleware func(next Handler) Handler

// Job 中间件
type JobMiddleware func(next JobHandler) JobHandler

// Event 中间件
type EventMiddleware func(next func(context.Context, Event) error) func(context.Context, Event) error

// Cron 中间件
type CronMiddleware func(next func(context.Context) error) func(context.Context) error
```

#### 内置中间件

- **IdempotencyMiddleware**：基于 Redis 的幂等检查

#### 自定义中间件示例

```go
func LoggingMiddleware(next taskbus.Handler) taskbus.Handler {
    return func(ctx context.Context, msg taskbus.Message) error {
        log.Printf("处理中: %s", msg.Topic)
        start := time.Now()
        err := next(ctx, msg)
        log.Printf("完成，耗时 %v, 错误: %v", time.Since(start), err)
        return err
    }
}
```

### 配置参考

```go
cfg := taskbus.Config{
    Namespace: "my-service",  // 命名空间（小写字母/数字/连字符）
    
    MQ: taskbus.MQConfig{
        Provider: taskbus.MQProviderRabbitMQ,  // 或 MQProviderRedis
        RabbitMQ: taskbus.RabbitMQConfig{
            URI:                 "amqp://localhost:5672",
            Exchange:            "app.events",
            DelayedExchange:     "app.events.delayed",  // 延迟消息交换机
            Prefetch:            64,
            ConsumerConcurrency: 4,
            DelayMode:           taskbus.DelayModeStandard,  // 或 DelayModeAliyun
        },
    },
    
    Job: taskbus.JobConfig{
        DefaultGroup: "default",
        Retry: taskbus.RetryConfig{
            Base:       time.Second,
            Factor:     2.0,
            MaxRetries: 3,
        },
    },
    
    Cron: taskbus.CronConfig{
        Distributed: true,                    // 启用分布式调度
        Timezone:    "Asia/Shanghai",
        LeaderTTL:   30 * time.Second,
    },
    
    // 幂等配置（可选）
    Idempotency: taskbus.IdempotencyConfig{
        RedisAddr: "localhost:6379",
        Prefix:    "idem",
        TTL:       24 * time.Hour,
    },
}
```

### 示例

查看 [example](./example) 目录获取完整示例：

- `example/jobs` - Jobs 任务队列示例
- `example/cron` - Cron 定时任务示例
- `example/eventbus` - EventBus 事件总线示例
- `example/service-isolation` - 多服务命名空间隔离示例

### 测试

```bash
go test -v ./...
go test -cover ./...
```

---

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.