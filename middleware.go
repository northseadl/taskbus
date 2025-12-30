package taskbus

import "context"

// Middleware 是 MQ Handler 的中间件类型。
// 用于在消息处理前后添加通用逻辑（如日志、监控、错误处理）。
//
// Example:
//
//	func LoggingMiddleware(next taskbus.Handler) taskbus.Handler {
//	    return func(ctx context.Context, msg taskbus.Message) error {
//	        log.Printf("Processing message: %s", msg.Topic)
//	        err := next(ctx, msg)
//	        if err != nil {
//	            log.Printf("Error: %v", err)
//	        }
//	        return err
//	    }
//	}
type Middleware func(next Handler) Handler

// JobMiddleware 是 Job 执行的中间件类型。
// 用于在任务执行前后添加通用逻辑。
//
// Example:
//
//	func MetricsMiddleware(next taskbus.JobHandler) taskbus.JobHandler {
//	    return func(ctx context.Context, jobName string, payload []byte) error {
//	        start := time.Now()
//	        err := next(ctx, jobName, payload)
//	        metrics.RecordJobDuration(jobName, time.Since(start))
//	        return err
//	    }
//	}
type JobMiddleware func(next JobHandler) JobHandler

// EventMiddleware 是 EventBus 事件处理的中间件类型。
type EventMiddleware func(next func(context.Context, Event) error) func(context.Context, Event) error

// CronMiddleware 是 Cron 任务的中间件类型。
//
// Example:
//
//	func RecoveryMiddleware(next func(context.Context) error) func(context.Context) error {
//	    return func(ctx context.Context) error {
//	        defer func() {
//	            if r := recover(); r != nil {
//	                log.Printf("Cron task panicked: %v", r)
//	            }
//	        }()
//	        return next(ctx)
//	    }
//	}
type CronMiddleware func(next func(context.Context) error) func(context.Context) error
