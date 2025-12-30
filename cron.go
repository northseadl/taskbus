package taskbus

import "context"

// Cron 提供基于 Cron 表达式的任务调度。
// 支持本地模式和分布式模式（通过 MQ 协调）。
//
// Cron 表达式格式（6 位，含秒）:
//
//	┌──────────── 秒 (0-59)
//	│ ┌────────── 分 (0-59)
//	│ │ ┌──────── 时 (0-23)
//	│ │ │ ┌────── 日 (1-31)
//	│ │ │ │ ┌──── 月 (1-12)
//	│ │ │ │ │ ┌── 周 (0-6, 0=周日)
//	│ │ │ │ │ │
//	* * * * * *
//
// Example:
//
//	// 每分钟执行
//	cli.Cron().Add("0 * * * * *", "cleanup", func(ctx context.Context) error {
//	    return doCleanup()
//	})
//
//	// 每天凌晨 2 点执行
//	cli.Cron().Add("0 0 2 * * *", "daily-report", generateDailyReport)
type Cron interface {
	// Add 添加一个 Cron 任务。
	// spec 为 6 位 Cron 表达式（含秒）。
	// name 为任务名称，用于标识和日志。
	// fn 为任务执行函数。
	// 返回任务 ID（用于 Remove）。
	Add(spec string, name string, fn func(context.Context) error, mws ...CronMiddleware) (id string, err error)

	// Remove 移除一个 Cron 任务。
	Remove(id string) error

	// Start 启动调度器。
	Start(ctx context.Context) error

	// Stop 停止调度器。
	Stop(ctx context.Context) error
}

// ---- no-op 实现 ----

type noopCron struct{}

func newNoopCron() Cron { return noopCron{} }

func (noopCron) Add(spec, name string, fn func(context.Context) error, mws ...CronMiddleware) (string, error) {
	return "", nil
}
func (noopCron) Remove(id string) error          { return nil }
func (noopCron) Start(ctx context.Context) error { return nil }
func (noopCron) Stop(ctx context.Context) error  { return nil }
