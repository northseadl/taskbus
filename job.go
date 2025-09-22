package taskbus

import (
	"context"
	"time"
)

// Job 定义业务任务。
type Job interface {
	Name() string
	Execute(ctx context.Context, payload []byte) error
}

// Jobs 提供任务入队与 Worker 管理。
type Jobs interface {
	Register(job Job)
	Enqueue(ctx context.Context, jobName string, payload []byte, opts ...EnqueueOption) error
	StartWorkers(ctx context.Context, groups map[string]int, mws ...JobMiddleware) (stop func(context.Context) error, err error)
}

// JobHandler 用于中间件包装 Job 执行。
type JobHandler func(ctx context.Context, jobName string, payload []byte) error

// EnqueueOption 入队选项。
type EnqueueOption func(*enqueueOpts)

type enqueueOpts struct {
	delay time.Duration
	key   string
}

// WithDelay 指定延时。
func WithDelay(d time.Duration) EnqueueOption { return func(o *enqueueOpts) { o.delay = d } }

// WithKey 指定幂等键或业务键。
func WithKey(k string) EnqueueOption { return func(o *enqueueOpts) { o.key = k } }

// ---- no-op 实现 ----

type noopJobs struct{}

func newNoopJobs() Jobs { return noopJobs{} }

func (noopJobs) Register(job Job) {}
func (noopJobs) Enqueue(ctx context.Context, jobName string, payload []byte, opts ...EnqueueOption) error {
	return nil
}
func (noopJobs) StartWorkers(ctx context.Context, groups map[string]int, mws ...JobMiddleware) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
