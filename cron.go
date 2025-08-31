package taskbus

import "context"

// Cron 提供基于 Cron 表达式的任务调度管理。
type Cron interface {
	Add(spec string, name string, fn func(context.Context) error, mws ...CronMiddleware) (id string, err error)
	Remove(id string) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ---- no-op 实现 ----

type noopCron struct{}

func newNoopCron() Cron { return noopCron{} }

func (noopCron) Add(spec, name string, fn func(context.Context) error, mws ...CronMiddleware) (string, error) { return "", nil }
func (noopCron) Remove(id string) error                                                                       { return nil }
func (noopCron) Start(ctx context.Context) error                                                               { return nil }
func (noopCron) Stop(ctx context.Context) error                                                                { return nil }

