package taskbus

// Package taskbus 提供统一的任务队列、定时调度、发布订阅与事件总线能力，
// 通过可插拔的 MQ 适配器（RabbitMQ/Redis）实现即时与延时消息、重试与中间件。
// 提供 Jobs、EventBus 与 Cron 三大子系统，并内置幂等中间件与延迟消息支持。
