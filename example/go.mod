module taskbus-example

go 1.24.6

require github.com/northseadl/taskbus v0.3.0

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
	github.com/redis/go-redis/v9 v9.12.1 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
)

replace github.com/northseadl/taskbus => ..
