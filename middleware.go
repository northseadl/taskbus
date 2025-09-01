package taskbus

import "context"

// Middleware 用于 MQ Handler。
type Middleware func(next Handler) Handler

// JobMiddleware 用于 Job 执行。
type JobMiddleware func(next JobHandler) JobHandler

// EventMiddleware 用于 EventBus。
type EventMiddleware func(next func(context.Context, Event) error) func(context.Context, Event) error

// CronMiddleware 用于 Cron 任务。
type CronMiddleware func(next func(context.Context) error) func(context.Context) error
