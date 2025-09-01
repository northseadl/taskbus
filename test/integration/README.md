# TaskBus 集成测试模块

这是 TaskBus 项目的独立集成测试模块，支持完全独立的发布和本地替换测试。

## 模块特性

- **独立模块**：拥有独立的 `go.mod` 文件，可以独立管理依赖
- **版本测试**：支持测试不同版本的 TaskBus 库
- **本地开发**：支持本地替换模式进行开发测试
- **Docker 环境**：包含完整的 RabbitMQ + Redis 测试环境

## 使用方式

### 1. 测试已发布版本

```bash
# 直接运行集成测试（使用 go.mod 中指定的版本）
cd test/integration
go test -v

# 测试特定版本
go mod edit -require=github.com/northseadl/taskbus@v0.1.0
go test -v
```

### 2. 本地开发测试

```bash
# 启用本地替换模式
cd test/integration
go mod edit -replace=github.com/northseadl/taskbus=../..
go mod tidy
go test -v
```

### 3. Docker 环境测试

```bash
# 启动测试环境
./run_integration.sh

# 或手动启动
docker compose -f docker-compose.test.yml up -d
export TQ_RABBITMQ_URI=amqp://admin:admin123@localhost:5673/
export TQ_RABBITMQ_EXCHANGE=taskbus.events
export TQ_RABBITMQ_DELAYED_EXCHANGE=taskbus.events.delayed
export TQ_REDIS_ADDR=localhost:6380
go test -v
```

## 测试场景

- **分布式 Cron**：Leader 选举与任务调度
- **事件总线**：多事件类型过滤功能
- **幂等性**：Jobs 和 EventBus 的 Redis KV 幂等性
- **端到端流程**：RabbitMQ 基础发布/消费
- **重试机制**：Jobs 重试与死信处理
- **并发处理**：RabbitMQ 并发消费验证
- **延时消息**：Redis 延时消息完整链路
- **连接恢复**：连接恢复与消息不丢失

## 环境要求

- Go 1.21+
- Docker & Docker Compose
- 可选：RabbitMQ 和 Redis 服务

## 配置说明

### 环境变量

- `TQ_RABBITMQ_URI`: RabbitMQ 连接地址
- `TQ_RABBITMQ_EXCHANGE`: RabbitMQ 交换机名称
- `TQ_RABBITMQ_DELAYED_EXCHANGE`: RabbitMQ 延迟消息交换机
- `TQ_REDIS_ADDR`: Redis 连接地址

### Docker 服务

- **RabbitMQ**: 端口 5673 (管理界面 15673)，包含延迟消息插件
- **Redis**: 端口 6380

## 版本管理

集成测试模块支持以下版本管理策略：

1. **固定版本测试**：在 CI/CD 中测试特定发布版本
2. **本地开发**：使用 replace 指令测试本地修改
3. **兼容性测试**：测试不同版本的 TaskBus 库兼容性

## 注意事项

- 集成测试需要真实的 Docker 环境，不使用模拟数据
- 测试严格遵循测试原则，验证端到端功能
- 所有测试都有适当的超时和错误处理
- 支持并行运行多个测试实例（通过不同端口）
