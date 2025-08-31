# TaskBus

高性能、生产可用的 Go 任务队列与事件总线库，支持 RabbitMQ/Redis、多中间件、延迟消息、幂等、重试/死信，以及本地/分布式 Cron 调度。

## 特性
- MQ 适配：RabbitMQ、Redis（即时/延迟消息）
- Jobs：重试、指数回退、死信
- EventBus：发布/订阅、类型过滤
- Cron：本地/分布式执行（基于 MQ）
- 幂等：内置 Redis 幂等中间件
- 生产就绪：日志接口、错误处理、可观察性留钩

## 安装

```
go get github.com/northseadl/taskbus
```

## 快速开始

````go
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
    _ = c.EventBus().Publish(ctx, taskbus.Event{
        Topic:   "user.created",
        Type:    "UserCreated",
        Subject: "uid-1",
        Payload: []byte("{}"),
    })
}
````

## 测试

- 单元测试（包内白盒）已清理，避免与生产代码耦合
- 集成测试放在 `test/integration`，通过环境变量控制：
  - RabbitMQ：`TQ_RABBITMQ_URI`、`TQ_RABBITMQ_EXCHANGE`、`TQ_RABBITMQ_DELAYED_EXCHANGE`
  - Redis：`TQ_REDIS_ADDR`

运行：
```
cd pkg/taskbus
go test ./test/integration -v
```

## 许可

MIT License

