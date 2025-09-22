package taskbus

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
)

type jobs struct {
	c   *client
	reg sync.Map // name -> Job
}

func newJobs(c *client) Jobs { return &jobs{c: c} }

func (j *jobs) Register(job Job) {
	if job != nil {
		j.reg.Store(job.Name(), job)
	}
}

func (j *jobs) Enqueue(ctx context.Context, jobName string, payload []byte, opts ...EnqueueOption) error {
	if jobName == "" {
		return fmt.Errorf("job name empty")
	}
	o := &enqueueOpts{}
	for _, fn := range opts {
		fn(o)
	}
	headers := map[string]string{"x-attempt": "0"}
	msg := Message{Topic: j.topic(jobName), Key: o.key, Body: payload, Headers: headers}
	if o.delay > 0 {
		return j.c.mq.PublishDelay(ctx, msg, o.delay)
	}
	return j.c.mq.Publish(ctx, msg)
}

func (j *jobs) StartWorkers(ctx context.Context, groups map[string]int, mws ...JobMiddleware) (func(context.Context) error, error) {
	if len(groups) == 0 {
		groups = map[string]int{"default": 1}
	}
	stops := make([]func(context.Context) error, 0, len(groups))
	// 默认接入重试中间件
	mqmw := j.retryMiddleware()
	wrapped := j.wrap(mws...)
	finalMW := func(next Handler) Handler { return mqmw(wrapped(next)) }
	for group := range groups {
		// 使用 RabbitMQ topic 通配符 job.#
		stop, err := j.c.mq.Consume(ctx, "job.#", group, j.handle, finalMW)
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

func (j *jobs) topic(name string) string { return "job." + name }

func (j *jobs) handle(ctx context.Context, msg Message) error {
	// 从 topic 提取 jobName: job.<name>
	name := msg.Topic
	if len(name) > 4 && name[:4] == "job." {
		name = name[4:]
	}
	v, ok := j.reg.Load(name)
	if !ok {
		return fmt.Errorf("job not registered: %s", name)
	}
	job := v.(Job)
	return job.Execute(ctx, msg.Body)
}

// retryMiddleware 在 MQ 层处理 Job 执行失败后的延时重发。
// 注意：为了避免与 MQ 层面的重试（Nack 重投）冲突，本中间件在捕获到错误后总是返回 nil，
// 让 MQ 消费端执行 Ack，从而仅通过延时重发（PublishDelay）实现指数回退重试。
func (j *jobs) retryMiddleware() Middleware {
	policy := ExponentialBackoff{Base: j.c.cfg.Job.Retry.Base, Factor: j.c.cfg.Job.Retry.Factor, MaxRetries: j.c.cfg.Job.Retry.MaxRetries}
	return func(next Handler) Handler {
		return func(ctx context.Context, m Message) error {
			err := next(ctx, m)
			if err == nil {
				return nil
			}
			attempt := 0
			if m.Headers != nil {
				if s, ok := m.Headers["x-attempt"]; ok {
					if n, e := strconv.Atoi(s); e == nil {
						attempt = n
					}
				}
			}
			if d, cont := policy.NextBackoff(attempt); cont {
				// 重发到原 topic，attempt+1
				h := copyHeaders(m.Headers)
				h["x-attempt"] = strconv.Itoa(attempt + 1)
				_ = j.c.mq.PublishDelay(ctx, Message{Topic: m.Topic, Key: m.Key, Body: m.Body, Headers: h}, d)
				return nil // 吞掉错误，避免 MQ Nack 重投
			}
			// 路由到死信
			if dl := j.c.cfg.Job.DeadLetterTopic; dl != "" {
				_ = j.c.mq.Publish(ctx, Message{Topic: dl, Key: m.Key, Body: m.Body, Headers: map[string]string{"x-dead": time.Now().Format(time.RFC3339)}})
			}
			return nil // 吞掉错误，Ack 消息
		}
	}
}

func (j *jobs) wrap(mws ...JobMiddleware) Middleware {
	return func(next Handler) Handler {
		// 转换为 JobHandler 中间件以复用
		final := func(ctx context.Context, jobName string, payload []byte) error {
			return j.handle(ctx, Message{Topic: j.topic(jobName), Body: payload})
		}
		for i := len(mws) - 1; i >= 0; i-- {
			final = mws[i](final)
		}
		return func(ctx context.Context, m Message) error {
			// topic -> jobName
			name := m.Topic
			if len(name) > 4 && name[:4] == "job." {
				name = name[4:]
			}
			return final(ctx, name, m.Body)
		}
	}
}

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
