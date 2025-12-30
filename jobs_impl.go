package taskbus

import (
	"context"
	"fmt"
	"sync"
)

// jobs 实现 Jobs 接口，提供任务注册、入队与 Worker 管理。
type jobs struct {
	c   *client
	reg sync.Map // name -> Job
}

func newJobs(c *client) Jobs { return &jobs{c: c} }

// Register 注册一个 Job 实现。
func (j *jobs) Register(job Job) {
	if job != nil {
		j.reg.Store(job.Name(), job)
	}
}

// Enqueue 将任务入队。
func (j *jobs) Enqueue(ctx context.Context, jobName string, payload []byte, opts ...EnqueueOption) error {
	if jobName == "" {
		return fmt.Errorf("job name empty")
	}
	o := &enqueueOpts{}
	for _, fn := range opts {
		fn(o)
	}
	headers := map[string]string{"x-retry-count": "0"}
	msg := Message{Topic: j.topic(jobName), Key: o.key, Body: payload, Headers: headers}
	if o.delay > 0 {
		return j.c.mq.PublishDelay(ctx, msg, o.delay)
	}
	return j.c.mq.Publish(ctx, msg)
}

// StartWorkers 启动 Worker 消费任务。
func (j *jobs) StartWorkers(ctx context.Context, groups map[string]int, mws ...JobMiddleware) (func(context.Context) error, error) {
	resolved := j.normalizeGroups(groups)
	stops := make([]func(context.Context) error, 0, len(resolved))

	// 构建中间件包装的 handler
	wrappedHandler := j.buildHandler(mws...)

	for group := range resolved {
		wildcard := buildWildcardTopic(j.c.namespace, "job")
		stop, err := j.c.mq.Consume(ctx, wildcard, group, wrappedHandler)
		if err != nil {
			return nil, err
		}
		stops = append(stops, stop)
	}
	return func(ctx context.Context) error {
		for _, s := range stops {
			_ = s(ctx)
		}
		return nil
	}, nil
}

// topic 构建任务的完整 topic 名称。
func (j *jobs) topic(name string) string {
	return buildTopic(j.c.namespace, "job", name)
}

// extractJobName 从 topic 中提取 job 名称。
func (j *jobs) extractJobName(topic string) string {
	prefix := buildTopicPrefix(j.c.namespace, "job")
	name := trimTopicPrefix(topic, prefix)
	// 兼容旧前缀 "job.<name>"
	if name == topic && len(name) > 4 && name[:4] == "job." {
		return name[4:]
	}
	return name
}

// handle 执行 Job。
func (j *jobs) handle(ctx context.Context, msg Message) error {
	name := j.extractJobName(msg.Topic)
	v, ok := j.reg.Load(name)
	if !ok {
		return fmt.Errorf("job not registered: %s", name)
	}
	job := v.(Job)
	return job.Execute(ctx, msg.Body)
}

// buildHandler 构建带中间件的 Handler。
// 修复：正确链接中间件，不再忽略 next Handler。
func (j *jobs) buildHandler(mws ...JobMiddleware) Handler {
	// 最终执行的 JobHandler
	finalJobHandler := func(ctx context.Context, jobName string, payload []byte) error {
		v, ok := j.reg.Load(jobName)
		if !ok {
			return fmt.Errorf("job not registered: %s", jobName)
		}
		job := v.(Job)
		return job.Execute(ctx, payload)
	}

	// 逆序包装中间件
	for i := len(mws) - 1; i >= 0; i-- {
		finalJobHandler = mws[i](finalJobHandler)
	}

	// 返回 MQ Handler，将 Message 转换为 JobHandler 调用
	return func(ctx context.Context, m Message) error {
		jobName := j.extractJobName(m.Topic)
		return finalJobHandler(ctx, jobName, m.Body)
	}
}

func (j *jobs) normalizeGroups(groups map[string]int) map[string]int {
	if len(groups) == 0 {
		groups = map[string]int{"": 1}
	}
	resolved := make(map[string]int, len(groups))
	for name, size := range groups {
		final := j.resolveGroup(name)
		if final == "" {
			final = "default"
		}
		if size <= 0 {
			size = 1
		}
		if existing, ok := resolved[final]; ok {
			if size > existing {
				resolved[final] = size
			}
			continue
		}
		resolved[final] = size
	}
	return resolved
}

func (j *jobs) resolveGroup(name string) string {
	final := name
	if final == "" {
		final = j.c.cfg.Job.DefaultGroup
	}
	if final == "" {
		final = "default"
	}
	// 若未显式配置 GroupPrefix，使用命名空间作为前缀
	prefix := j.c.cfg.Job.GroupPrefix
	if prefix == "" {
		prefix = j.c.namespace
	}
	if prefix != "" {
		if final != "" {
			final = prefix + "." + final
		} else {
			final = prefix
		}
	}
	return final
}

// copyHeaders 复制 headers map。
func copyHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return map[string]string{}
	}
	m := make(map[string]string, len(h))
	for k, v := range h {
		m[k] = v
	}
	return m
}
