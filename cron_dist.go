package taskbus

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	cronv3 "github.com/robfig/cron/v3"
)

var redisLockReleaseScript = redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`)

// cronDist 基于 MQ 的分布式 Cron：Scheduler + Executor
// - Scheduler: 仅 Leader 实例运行，按 spec 将触发事件发布到 MQ
// - Executor: 全部实例订阅 cron 任务，按收到的任务名称执行本地注册的 fn
type cronDist struct {
	c   *client
	mu  sync.Mutex
	reg map[string]cronTask // name -> task

	// 生命周期控制
	stopCh       chan struct{}      // 全局停止信号
	leaderCtx    context.Context    // Leader 上下文
	leaderCancel context.CancelFunc // Leader 取消函数
	execStop     func(context.Context) error
	stopOnce     sync.Once

	// 调度器状态
	cron *cronv3.Cron
}

type cronTask struct {
	spec string
	fn   func(context.Context) error
	mws  []CronMiddleware
}

func newCron(c *client) Cron {
	if c.cfg.Cron.Distributed {
		return &cronDist{
			c:      c,
			reg:    map[string]cronTask{},
			stopCh: make(chan struct{}),
		}
	}
	return newCronLocal(c)
}

// Add 注册一个 Cron 任务。
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
		taskName := key // 闭包捕获
		wrapped := cd.publishFunc(taskName)
		if _, err := cd.cron.AddFunc(spec, func() { _ = wrapped(context.Background()) }); err != nil {
			return "", err
		}
	}
	return key, nil
}

// Remove 移除一个 Cron 任务。
func (cd *cronDist) Remove(id string) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.reg, id)
	// 简化：重建调度器
	if cd.cron != nil {
		cd.rebuildSchedulerLocked()
	}
	return nil
}

// Start 启动 Cron 服务。
func (cd *cronDist) Start(ctx context.Context) error {
	// 启动 Executor（所有实例）
	stop, err := cd.startExecutor(ctx)
	if err != nil {
		return err
	}
	cd.execStop = stop
	cd.c.logger.Info(ctx, "cron executor started", "group", cd.c.cfg.Cron.ExecutorGroup)

	// 启动 Leader 选举与 Scheduler（仅 Leader 实例）
	go cd.leaderLoop(ctx)
	return nil
}

// Stop 停止 Cron 服务。
func (cd *cronDist) Stop(ctx context.Context) error {
	// 发送停止信号
	cd.stopOnce.Do(func() { close(cd.stopCh) })

	// 取消 Leader 上下文
	cd.mu.Lock()
	if cd.leaderCancel != nil {
		cd.leaderCancel()
	}
	cd.mu.Unlock()

	// 停止 Executor
	if cd.execStop != nil {
		_ = cd.execStop(ctx)
	}
	return nil
}

// --- Leader 选举 ---

func (cd *cronDist) leaderLoop(ctx context.Context) {
	for {
		select {
		case <-cd.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		// 竞争成为 leader
		leaderCtx, cleanup := cd.tryAcquireLeader(ctx)
		if leaderCtx != nil {
			cd.mu.Lock()
			cd.leaderCtx = leaderCtx
			cd.mu.Unlock()

			cd.c.logger.Info(leaderCtx, "cron leader acquired")
			cd.startScheduler(leaderCtx)

			// 等待 Leader 上下文结束
			<-leaderCtx.Done()

			cd.stopScheduler()
			if cleanup != nil {
				cleanup()
			}
			cd.c.logger.Info(context.Background(), "cron leader released")
		}

		// 等待后重试
		select {
		case <-cd.stopCh:
			return
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// tryAcquireLeader 尝试获取 Leader 锁。
// 返回 Leader 上下文和清理函数；失败返回 nil, nil。
func (cd *cronDist) tryAcquireLeader(parentCtx context.Context) (context.Context, func()) {
	switch cd.c.cfg.MQ.Provider {
	case MQProviderRabbitMQ:
		return cd.tryLeaderWithRabbit(parentCtx)
	case MQProviderRedis:
		return cd.tryLeaderWithRedis(parentCtx)
	default:
		return nil, nil
	}
}

func (cd *cronDist) tryLeaderWithRabbit(parentCtx context.Context) (context.Context, func()) {
	uri := cd.c.cfg.MQ.RabbitMQ.URI
	conn, err := amqp.Dial(uri)
	if err != nil {
		return nil, nil
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil
	}

	// 独占队列作为 leader 锁（基于 namespace 隔离）
	qname := "taskbus.cron.leader." + cd.c.namespace
	_, err = ch.QueueDeclare(qname, false, true, true, true, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil
	}

	// 创建 Leader 上下文
	leaderCtx, cancel := context.WithCancel(parentCtx)
	cd.mu.Lock()
	cd.leaderCancel = cancel
	cd.mu.Unlock()

	// 监听连接关闭
	closeChan := conn.NotifyClose(make(chan *amqp.Error, 1))
	go func() {
		select {
		case <-leaderCtx.Done():
		case <-closeChan:
			cancel() // 连接断开时取消 Leader
		}
	}()

	cleanup := func() {
		_ = ch.Close()
		_ = conn.Close()
	}

	return leaderCtx, cleanup
}

func (cd *cronDist) tryLeaderWithRedis(parentCtx context.Context) (context.Context, func()) {
	rc := cd.newRedisClient()
	if rc == nil {
		return nil, nil
	}

	key := cd.c.cfg.Cron.LeaderLockKey
	if key == "" {
		key = "taskbus:" + cd.c.namespace + ":cron:leader"
	}
	ttl := cd.c.cfg.Cron.LeaderTTL
	if ttl <= 0 {
		ttl = 10 * time.Second
	}

	// 生成唯一标识
	leaderID := fmt.Sprintf("%s-%d-%d", hostname(), os.Getpid(), time.Now().UnixNano())

	ok, _ := rc.SetNX(parentCtx, key, leaderID, ttl).Result()
	if !ok {
		_ = rc.Close()
		return nil, nil
	}

	// 创建 Leader 上下文
	leaderCtx, cancel := context.WithCancel(parentCtx)
	cd.mu.Lock()
	cd.leaderCancel = cancel
	cd.mu.Unlock()

	// 启动续租协程
	go func() {
		defer rc.Close()
		ticker := time.NewTicker(ttl / 3) // 更频繁的续租
		defer ticker.Stop()
		for {
			select {
			case <-leaderCtx.Done():
				// 释放锁（仅当仍持有）
				_ = releaseRedisLock(context.Background(), rc, key, leaderID)
				return
			case <-ticker.C:
				// 续租前检查是否仍是 Leader
				val, err := rc.Get(context.Background(), key).Result()
				if err != nil || val != leaderID {
					cancel() // 失去 Leader 身份
					return
				}
				_ = rc.Expire(context.Background(), key, ttl).Err()
			}
		}
	}()

	return leaderCtx, nil
}

func (cd *cronDist) newRedisClient() *redis.Client {
	addr := cd.c.cfg.MQ.Redis.Addr
	if addr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: cd.c.cfg.MQ.Redis.Username,
		Password: cd.c.cfg.MQ.Redis.Password,
		DB:       cd.c.cfg.MQ.Redis.DB,
	})
}

func releaseRedisLock(ctx context.Context, rc *redis.Client, key, value string) error {
	if rc == nil {
		return nil
	}
	return redisLockReleaseScript.Run(ctx, rc, []string{key}, value).Err()
}

// hostname 返回主机名，失败返回 "unknown"。
func hostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "unknown"
}

// --- 调度器 ---

func (cd *cronDist) startScheduler(ctx context.Context) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	loc := time.Local
	if tz := cd.c.cfg.Cron.Timezone; tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	cd.cron = cronv3.New(cronv3.WithSeconds(), cronv3.WithLocation(loc))

	for name, t := range cd.reg {
		taskName := name // 闭包捕获
		wrapped := cd.publishFunc(taskName)
		_, _ = cd.cron.AddFunc(t.spec, func() { _ = wrapped(ctx) })
	}
	cd.cron.Start()
	cd.c.logger.Info(ctx, "cron scheduler started", "task_count", len(cd.reg))
}

func (cd *cronDist) stopScheduler() {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	if cd.cron != nil {
		cd.cron.Stop()
		cd.cron = nil
	}
}

// rebuildSchedulerLocked 重建调度器（需持有锁）。
func (cd *cronDist) rebuildSchedulerLocked() {
	if cd.cron != nil {
		cd.cron.Stop()
		cd.cron = nil
		// 使用当前 Leader 上下文重建
		ctx := cd.leaderCtx
		if ctx == nil {
			ctx = context.Background()
		}
		cd.mu.Unlock()
		cd.startScheduler(ctx)
		cd.mu.Lock()
	}
}

func (cd *cronDist) publishFunc(name string) func(context.Context) error {
	return func(ctx context.Context) error {
		topic := buildTopic(cd.c.namespace, "cron", name)
		// 必须提供非空消息体（某些 RabbitMQ 实现如阿里云 Serverless 会拒绝空消息）
		body := []byte("{}")
		if err := cd.c.mq.Publish(ctx, Message{Topic: topic, Key: name, Body: body}); err != nil {
			cd.c.logger.Error(ctx, "cron task publish failed", "task", name, "error", err.Error())
			return err
		}
		return nil
	}
}

// --- 执行器 ---

func (cd *cronDist) startExecutor(ctx context.Context) (func(context.Context) error, error) {
	group := cd.c.cfg.Cron.ExecutorGroup
	if group == "" {
		group = cd.c.namespace + ".cron-exec"
	}
	wildcard := buildWildcardTopic(cd.c.namespace, "cron")
	stopW, err := cd.c.mq.Consume(ctx, wildcard, group, cd.execHandle)
	if err != nil {
		return nil, fmt.Errorf("cron executor consume failed: %w", err)
	}
	return stopW, nil
}

func (cd *cronDist) execHandle(ctx context.Context, m Message) error {
	prefix := buildTopicPrefix(cd.c.namespace, "cron")
	name := trimTopicPrefix(m.Topic, prefix)

	cd.mu.Lock()
	t, ok := cd.reg[name]
	cd.mu.Unlock()
	if !ok {
		return fmt.Errorf("cron task not found: %s", name)
	}

	// 组装中间件链
	fn := t.fn
	for i := len(t.mws) - 1; i >= 0; i-- {
		fn = t.mws[i](fn)
	}
	return fn(ctx)
}
