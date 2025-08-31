package taskbus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisAdapter 基于 Redis Streams 实现 MQ；延时消息通过 ZSET 调度器转存至 Streams。

type redisAdapter struct {
	rdb    *redis.Client
	cfg    RedisConfig
	logger Logger

	stopCh chan struct{}
	wg     sync.WaitGroup
}

const (
	redisDelayZKey = "tq:delay"
)

type delayItem struct {
	Topic   string            `json:"topic"`
	Key     string            `json:"key"`
	BodyB64 string            `json:"body_b64"`
	Headers map[string]string `json:"headers"`
}

func newRedisAdapter(cfg RedisConfig, logger Logger) (MQ, error) {
	if cfg.Addr == "" { return nil, fmt.Errorf("redis addr empty") }
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Addr, Username: cfg.Username, Password: cfg.Password, DB: cfg.DB})
	ad := &redisAdapter{cfg: cfg, logger: logger, rdb: rdb, stopCh: make(chan struct{})}
	ad.startDelayScheduler()
	return ad, nil
}

func (r *redisAdapter) startDelayScheduler() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ctx := context.Background()
		for {
			select {
			case <-r.stopCh:
				return
			case <-time.After(200 * time.Millisecond):
				now := float64(time.Now().UnixMilli())
				items, err := r.rdb.ZRangeByScore(ctx, redisDelayZKey, &redis.ZRangeBy{Min: "-inf", Max: fmt.Sprintf("%f", now), Offset: 0, Count: 100}).Result()
				if err != nil { continue }
				for _, s := range items {
					var di delayItem
					if json.Unmarshal([]byte(s), &di) == nil {
						body, _ := base64.StdEncoding.DecodeString(di.BodyB64)
						_ = r.publishStream(ctx, Message{Topic: di.Topic, Key: di.Key, Body: body, Headers: di.Headers})
					}
					_, _ = r.rdb.ZRem(ctx, redisDelayZKey, s).Result()
				}
			}
		}
	}()
}

func (r *redisAdapter) Publish(ctx context.Context, msg Message) error {
	return r.publishStream(ctx, msg)
}

func (r *redisAdapter) PublishDelay(ctx context.Context, msg Message, delay time.Duration) error {
	di := delayItem{Topic: msg.Topic, Key: msg.Key, BodyB64: base64.StdEncoding.EncodeToString(msg.Body), Headers: msg.Headers}
	b, _ := json.Marshal(di)
	score := float64(time.Now().Add(delay).UnixMilli())
	return r.rdb.ZAdd(ctx, redisDelayZKey, redis.Z{Score: score, Member: string(b)}).Err()
}

func (r *redisAdapter) Consume(ctx context.Context, topic, group string, handler Handler, mws ...Middleware) (func(context.Context) error, error) {
	if group == "" { group = "default" }
	// 确保 group 存在，使用 "0" 从头开始读取
	_ = r.rdb.XGroupCreateMkStream(ctx, topic, group, "0").Err()
	final := handler
	for i := len(mws) - 1; i >= 0; i-- { final = mws[i](final) }

	done := make(chan struct{})
	cctx, cancel := context.WithCancel(ctx)
	r.wg.Add(1)
	go func() {
		defer func() { r.wg.Done(); close(done) }()
		concurrency := r.cfg.ConsumerConcurrency
		if concurrency <= 0 { concurrency = 1 }
		sem := make(chan struct{}, concurrency)
		for {
			select {
			case <-cctx.Done():
				return
			default:
			}
			// BLOCK 2s 读取
			res, err := r.rdb.XReadGroup(cctx, &redis.XReadGroupArgs{
				Group:    group,
				Consumer: fmt.Sprintf("%s-%d", group, time.Now().UnixNano()),
				Streams:  []string{topic, ">"},
				Count:    int64(concurrency),
				Block:    2 * time.Second,
			}).Result()
			if err == redis.Nil || (err != nil && cctx.Err() != nil) { continue }
			if err != nil { continue }
			for _, str := range res {
				for _, xmsg := range str.Messages {
					sem <- struct{}{}
					go func(m redis.XMessage) {
						defer func() { <-sem }()
						msg := r.decodeXMessage(topic, m)
						_ = final(cctx, msg)
						_, _ = r.rdb.XAck(cctx, topic, group, m.ID).Result()
					}(xmsg)
				}
			}
		}
	}()
	stop := func(sctx context.Context) error { cancel(); select { case <-done: return nil; case <-sctx.Done(): return sctx.Err() } }
	return stop, nil
}

func (r *redisAdapter) Close(ctx context.Context) error {
	close(r.stopCh)
	done := make(chan struct{})
	go func(){ r.wg.Wait(); close(done) }()
	select { case <-done: default: }
	return r.rdb.Close()
}

func (r *redisAdapter) publishStream(ctx context.Context, msg Message) error {
	fields := map[string]interface{}{"key": msg.Key, "body": base64.StdEncoding.EncodeToString(msg.Body)}
	for k, v := range msg.Headers { fields["h:"+k] = v }
	return r.rdb.XAdd(ctx, &redis.XAddArgs{Stream: msg.Topic, Values: fields}).Err()
}

func (r *redisAdapter) decodeXMessage(topic string, xm redis.XMessage) Message {
	var key string
	var body []byte
	headers := make(map[string]string)
	for k, v := range xm.Values {
		switch k {
		case "key": key, _ = v.(string)
		case "body":
			if s, ok := v.(string); ok { body, _ = base64.StdEncoding.DecodeString(s) }
		default:
			if len(k) > 2 && k[:2] == "h:" { if s, ok := v.(string); ok { headers[k[2:]] = s } }
		}
	}
	return Message{Topic: topic, Key: key, Body: body, Headers: headers}
}

