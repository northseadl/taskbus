#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/integration"
COMPOSE_FILE=docker-compose.test.yml

# Start fresh env
docker compose -f "$COMPOSE_FILE" down --remove-orphans || true
docker compose -f "$COMPOSE_FILE" up -d --remove-orphans

# Wait healthy
echo "[+] Waiting for services to be healthy..."
for i in {1..40}; do
  RQ=$(docker inspect -f '{{.State.Health.Status}}' taskbus-rabbitmq-test || echo starting)
  RS=$(docker inspect -f '{{.State.Health.Status}}' taskbus-redis-test || echo starting)
  echo "rabbitmq=$RQ redis=$RS"
  [[ "$RQ" == "healthy" && "$RS" == "healthy" ]] && break
  sleep 3
  if [[ $i -eq 40 ]]; then echo "services not healthy"; exit 1; fi
done

echo "[+] Verifying delayed plugin..."
docker exec taskbus-rabbitmq-test rabbitmq-plugins list | grep delayed || echo "[WARN] delayed plugin not listed"

# Env
export TQ_RABBITMQ_URI=${TQ_RABBITMQ_URI:-amqp://admin:admin123@localhost:5672/}
export TQ_RABBITMQ_EXCHANGE=${TQ_RABBITMQ_EXCHANGE:-taskbus.events}
export TQ_RABBITMQ_DELAYED_EXCHANGE=${TQ_RABBITMQ_DELAYED_EXCHANGE:-taskbus.events.delayed}
export TQ_REDIS_ADDR=${TQ_REDIS_ADDR:-localhost:6379}

echo "[+] Running integration tests"
set -x
go test ../../test/integration -v
set +x

echo "[+] Done"
