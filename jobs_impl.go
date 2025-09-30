package taskbus

import (
	"context"
	"fmt"
	"sync"
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
	headers := map[string]string{"x-retry-count": "0"}
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
	wrapped := j.wrap(mws...)
	finalMW := func(next Handler) Handler { return wrapped(next) }
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
