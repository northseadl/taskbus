package taskbus

import (
	"context"
	"time"
)

// Job 定义业务任务接口。
// 实现此接口以注册可执行的任务。
//
// Example:
//
//	type SendEmailJob struct{}
//
//	func (SendEmailJob) Name() string { return "email.send" }
//
//	func (SendEmailJob) Execute(ctx context.Context, payload []byte) error {
//	    var req EmailRequest
//	    json.Unmarshal(payload, &req)
//	    return sendEmail(req)
//	}
type Job interface {
	// Name 返回任务名称，用于路由和注册。
	Name() string

	// Execute 执行任务。
	// payload 为任务载荷（通常是 JSON 序列化的业务数据）。
	Execute(ctx context.Context, payload []byte) error
}

// Jobs 提供任务队列功能：注册、入队和 Worker 管理。
type Jobs interface {
	// Register 注册一个 Job 实现。
	Register(job Job)

	// Enqueue 将任务入队。
	// jobName 必须与已注册的 Job.Name() 匹配。
	Enqueue(ctx context.Context, jobName string, payload []byte, opts ...EnqueueOption) error

	// StartWorkers 启动 Worker 消费任务。
	// groups 为消费组名称到并发数的映射。
	// 返回 stop 函数用于优雅停止。
	StartWorkers(ctx context.Context, groups map[string]int, mws ...JobMiddleware) (stop func(context.Context) error, err error)
}

// JobHandler 是 Job 执行函数签名，用于中间件包装。
type JobHandler func(ctx context.Context, jobName string, payload []byte) error

// EnqueueOption 是入队选项函数类型。
type EnqueueOption func(*enqueueOpts)

type enqueueOpts struct {
	delay time.Duration
	key   string
}

// WithDelay 设置延时入队。
// 任务将在 delay 时间后才会被消费。
//
// Example:
//
//	cli.Jobs().Enqueue(ctx, "email.send", payload, taskbus.WithDelay(5*time.Minute))
func WithDelay(d time.Duration) EnqueueOption {
	return func(o *enqueueOpts) { o.delay = d }
}

// WithKey 设置业务键，用于幂等检查。
// 相同 key 的任务在幂等窗口内只会执行一次。
//
// Example:
//
//	cli.Jobs().Enqueue(ctx, "order.process", payload, taskbus.WithKey("order-123"))
func WithKey(k string) EnqueueOption {
	return func(o *enqueueOpts) { o.key = k }
}

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
