# TaskBus

高性能、生产可用的 Go 任务队列与事件总线库，支持 RabbitMQ/Redis、多中间件、延迟消息、幂等、重试/死信，以及本地/分布式 Cron 调度。

## 特性
- **MQ 适配**：RabbitMQ、Redis（即时/延迟消息）
- **Jobs**：重试、指数回退、死信处理
- **EventBus**：发布/订阅、类型过滤
- **Cron**：本地/分布式执行（基于 MQ Leader 选举）
- **幂等**：内置 Redis 幂等中间件
- **生产就绪**：日志接口、错误处理、连接恢复

## 安装

```bash
go get github.com/northseadl/taskbus
```

## 快速开始

```go
package main

import (
    "context"
    "log"

    "github.com/northseadl/taskbus"
)

func main() {
    cfg := taskbus.Config{
        MQ: taskbus.MQConfig{
            Provider: taskbus.MQProviderRabbitMQ,
            RabbitMQ: taskbus.RabbitMQConfig{
                URI:      "amqp://localhost:5672",
                Exchange: "app.events",
            },
        },
    }

    ctx := context.Background()
    c, err := taskbus.New(ctx, cfg)
    if err != nil { log.Fatal(err) }
    defer c.Close(ctx)

    // 发布事件
    _ = c.Bus().Publish(ctx, taskbus.Event{
        Topic:   "user.created",
        Type:    "UserCreated",
        Subject: "uid-1",
        Payload: []byte("{}"),
    })
}
```

## 测试

### 集成测试矩阵

完整的端到端集成测试覆盖以下场景：

| 测试场景 | 文件 | 验证内容 |
|---------|------|---------|
| 基础消息流 | `integration_test.go` | RabbitMQ 发布/消费、Jobs 重试/死信 |
| 事件过滤 | `eventbus_filter_test.go` | FilterByType 多事件类型筛选 |
| Redis 延时 | `redis_delay_test.go` | PublishDelay → Stream 搬运 → 延时消费 |
| 分布式 Cron | `cron_distributed_test.go` | Leader 选举与任务调度 |
| 幂等性 | `idempotency_test.go` | Jobs/EventBus Redis KV 幂等验证 |
| 并发消费 | `rabbitmq_concurrency_test.go` | ConsumerConcurrency 参数验证 |
| 连接恢复 | `resilience_test.go` | 重连后消息不丢失 |

### 运行方式

**本地 Docker 环境**：
```bash
cd pkg/taskbus
chmod +x test/run_integration.sh
./test/run_integration.sh
```

**手动运行**（需先启动 RabbitMQ/Redis）：
```bash
export TQ_RABBITMQ_URI=amqp://admin:admin123@localhost:5673/
export TQ_RABBITMQ_EXCHANGE=taskbus.events
export TQ_RABBITMQ_DELAYED_EXCHANGE=taskbus.events.delayed
export TQ_REDIS_ADDR=localhost:6380

cd pkg/taskbus
go test ./test/integration -v
```

**环境变量门禁**：未设置环境变量时测试自动跳过，适合 CI/CD 流水线。

### GitHub Actions CI

项目配置了自动化 CI，在每次推送和 PR 时运行完整的集成测试矩阵。

## 许可

MIT License

