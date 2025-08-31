package taskbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	cronv3 "github.com/robfig/cron/v3"
)

// cronDist 基于 MQ 的分布式 Cron：Scheduler + Executor
// - Scheduler: 仅 Leader 实例运行，按 spec 将触发事件发布到 MQ (topic: cron.<name>)
// - Executor: 全部实例订阅 cron.#，按收到的任务名称执行本地注册的 fn

type cronDist struct {
	c   *client
	mu  sync.Mutex
	reg map[string]cronTask // name -> task

	leaderCancel context.CancelFunc
	execStop     func(context.Context) error

	// scheduler state
	cron   *cronv3.Cron
	lockCh chan struct{}
}

type cronTask struct {
	spec string
	fn   func(context.Context) error
	mws  []CronMiddleware
}

func newCron(c *client) Cron {
	if c.cfg.Cron.Distributed {
		return &cronDist{c: c, reg: map[string]cronTask{}, lockCh: make(chan struct{}, 1)}
	}
	return newCronLocal(c)
}

// --- Local impl kept in cron_impl_local.go ---

func (cd *cronDist) Add(spec string, name string, fn func(context.Context) error, mws ...CronMiddleware) (string, error) {
	if fn == nil {
		return "", fmt.Errorf("nil fn")
	}
	cd.mu.Lock()
	defer cd.mu.Unlock()
	key := name
	if key == "" {
		key = spec
	}
	cd.reg[key] = cronTask{spec: spec, fn: fn, mws: mws}
	// 如果当前是 Leader，动态注册到 scheduler
	if cd.cron != nil {
		wrapped := cd.publishFunc(key)
		if _, err := cd.cron.AddFunc(spec, func() { _ = wrapped(context.Background()) }); err != nil {
			return "", err
		}
	}
	return key, nil
}

func (cd *cronDist) Remove(id string) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.reg, id)
	// 简化：重建调度器
	if cd.cron != nil {
		cd.rebuildScheduler()
	}
	return nil
}

func (cd *cronDist) Start(ctx context.Context) error {
	// 启动 Executor（所有实例）
	stop, err := cd.startExecutor(ctx)
	if err != nil {
		return err
	}
	cd.execStop = stop
	// 启动 Leader 选举与 Scheduler（仅 Leader 实例）
	go cd.leaderLoop()
	return nil
}

func (cd *cronDist) Stop(ctx context.Context) error {
	if cd.leaderCancel != nil {
		cd.leaderCancel()
	}
	if cd.execStop != nil {
		_ = cd.execStop(ctx)
	}
	return nil
}

// --- Scheduler (Leader only) ---

func (cd *cronDist) leaderLoop() {
	for {
		// 竞争成为 leader
		if cd.tryAcquireLeader() {
			ctx, cancel := context.WithCancel(context.Background())
			cd.leaderCancel = cancel
			cd.startScheduler(ctx)
			<-ctx.Done()
			cd.stopScheduler()
		}
		time.Sleep(2 * time.Second)
	}
}

func (cd *cronDist) tryAcquireLeader() bool {
	// 优先使用 MQ Provider 的原生能力做锁
	switch cd.c.cfg.MQ.Provider {
	case MQProviderRabbitMQ:
		return cd.tryLeaderWithRabbit()
	case MQProviderRedis:
		return cd.tryLeaderWithRedis()
	default:
		return false
	}
}

func (cd *cronDist) tryLeaderWithRabbit() bool {
	uri := cd.c.cfg.MQ.RabbitMQ.URI
	conn, err := amqp.Dial(uri)
	if err != nil {
		return false
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return false
	}
	// 独占队列作为 leader 锁
	_, err = ch.QueueDeclare("tq.cron.leader", false, true, true, true, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return false
	}
	// 持有连接与通道直到取消
	ctx, cancel := context.WithCancel(context.Background())
	cd.leaderCancel = cancel
	go func() { <-ctx.Done(); _ = ch.Close(); _ = conn.Close() }()
	return true
}

func (cd *cronDist) tryLeaderWithRedis() bool {
	rc := cd.newRedisClient()
	if rc == nil {
		return false
	}
	ctx := context.Background()
	key := cd.c.cfg.Cron.LeaderLockKey
	if key == "" {
		key = "tq:cron:leader"
	}
	ttl := cd.c.cfg.Cron.LeaderTTL
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	ok, _ := rc.SetNX(ctx, key, cd.c.cfg.Logger.Level, ttl).Result()
	if !ok {
		_ = rc.Close()
		return false
	}
	ctx2, cancel := context.WithCancel(context.Background())
	cd.leaderCancel = cancel
	// 续租
	go func() {
		defer rc.Close()
		t := time.NewTicker(ttl / 2)
		for {
			select {
			case <-ctx2.Done():
				return
			case <-t.C:
				_ = rc.Expire(ctx, key, ttl).Err()
			}
		}
	}()
	return true
}

func (cd *cronDist) newRedisClient() *redis.Client {
	addr := cd.c.cfg.MQ.Redis.Addr
	if addr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{Addr: addr, Username: cd.c.cfg.MQ.Redis.Username, Password: cd.c.cfg.MQ.Redis.Password, DB: cd.c.cfg.MQ.Redis.DB})
}

func (cd *cronDist) startScheduler(ctx context.Context) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	loc := time.Local
	if tz := cd.c.cfg.Cron.Timezone; tz != "" {
		if l, err := time.LoadLocation(tz); err == nil { loc = l }
	}
	cd.cron = cronv3.New(cronv3.WithSeconds(), cronv3.WithLocation(loc))
	for name, t := range cd.reg {
		wrapped := cd.publishFunc(name)
		_, _ = cd.cron.AddFunc(t.spec, func() { _ = wrapped(context.Background()) })
	}
	cd.cron.Start()
}

func (cd *cronDist) stopScheduler() {
	if cd.cron != nil {
		cd.cron.Stop()
		cd.cron = nil
	}
}

func (cd *cronDist) rebuildScheduler() {
	if cd.cron != nil {
		cd.stopScheduler()
		cd.startScheduler(context.Background())
	}
}

func (cd *cronDist) publishFunc(name string) func(context.Context) error {
	return func(ctx context.Context) error {
		// 发布到 MQ: cron.<name>
		return cd.c.mq.Publish(ctx, Message{Topic: "cron." + name, Key: name, Body: nil})
	}
}

// --- Executor (All instances) ---

func (cd *cronDist) startExecutor(ctx context.Context) (func(context.Context) error, error) {
	group := cd.c.cfg.Cron.ExecutorGroup
	if group == "" {
		group = "cron-exec"
	}
	return cd.c.mq.Consume(ctx, "cron.#", group, cd.execHandle)
}

func (cd *cronDist) execHandle(ctx context.Context, m Message) error {
	name := m.Topic
	if len(name) > 5 && name[:5] == "cron." {
		name = name[5:]
	}
	cd.mu.Lock()
	t, ok := cd.reg[name]
	cd.mu.Unlock()
	if !ok {
		return fmt.Errorf("cron task not found: %s", name)
	}
	fn := t.fn
	for i := len(t.mws) - 1; i >= 0; i-- {
		fn = t.mws[i](fn)
	}
	return fn(ctx)
}
