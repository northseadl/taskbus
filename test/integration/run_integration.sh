#!/usr/bin/env bash
set -euo pipefail

# TaskBus 集成测试独立模块运行脚本

echo "=== TaskBus 集成测试独立模块 ==="
echo "当前目录: $(pwd)"
echo "模块名称: $(go list -m)"
echo "TaskBus 版本: $(go list -m github.com/northseadl/taskbus)"
echo

COMPOSE_FILE=docker-compose.test.yml

# Start fresh env
echo "[+] Starting Docker services..."
docker compose -f "$COMPOSE_FILE" down --remove-orphans || true
docker compose -f "$COMPOSE_FILE" up -d --remove-orphans

# Wait healthy
echo "[+] Waiting for services to be healthy..."
for i in {1..50}; do
  RQ=$(docker inspect -f '{{.State.Health.Status}}' taskbus-rabbitmq-test || echo starting)
  RS=$(docker inspect -f '{{.State.Health.Status}}' taskbus-redis-test || echo starting)
  echo "rabbitmq=$RQ redis=$RS"
  [[ "$RQ" == "healthy" && "$RS" == "healthy" ]] && break
  sleep 3
  if [[ $i -eq 50 ]]; then echo "services not healthy"; exit 1; fi
done

echo "[+] Verifying delayed plugin..."
docker exec taskbus-rabbitmq-test rabbitmq-plugins list | grep delayed || echo "[WARN] delayed plugin not listed"

# Set environment variables (use remapped ports)
export TQ_RABBITMQ_URI=${TQ_RABBITMQ_URI:-amqp://admin:admin123@localhost:5673/}
export TQ_RABBITMQ_EXCHANGE=${TQ_RABBITMQ_EXCHANGE:-taskbus.events}
export TQ_RABBITMQ_DELAYED_EXCHANGE=${TQ_RABBITMQ_DELAYED_EXCHANGE:-taskbus.events.delayed}
export TQ_REDIS_ADDR=${TQ_REDIS_ADDR:-localhost:6380}

echo "[+] Environment variables set:"
echo "  TQ_RABBITMQ_URI=$TQ_RABBITMQ_URI"
echo "  TQ_RABBITMQ_EXCHANGE=$TQ_RABBITMQ_EXCHANGE"
echo "  TQ_RABBITMQ_DELAYED_EXCHANGE=$TQ_RABBITMQ_DELAYED_EXCHANGE"
echo "  TQ_REDIS_ADDR=$TQ_REDIS_ADDR"

echo "[+] Running integration tests..."
set -x
go test . -v -timeout=15m
set +x

echo "[+] All integration tests completed."
echo
echo "💡 提示："
echo "- 本地开发模式: go mod edit -replace=github.com/northseadl/taskbus=../.."
echo "- 发布版本模式: go mod edit -dropreplace=github.com/northseadl/taskbus"
echo "- 多模式演示: ./test_modes.sh"
