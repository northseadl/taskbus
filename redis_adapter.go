package taskbus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisAdapter 基于 Redis Streams 实现 MQ；延时消息通过 ZSET 调度器转存至 Streams。

type redisAdapter struct {
	rdb          *redis.Client
	cfg          RedisConfig
	retry        RetryConfig
	logger       Logger
	consumerName string

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

func newRedisAdapter(cfg RedisConfig, retry RetryConfig, logger Logger) (MQ, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis addr empty")
	}
	consumerName := ""
	if hn, _ := os.Hostname(); hn != "" {
		consumerName = fmt.Sprintf("%s-%d", hn, os.Getpid())
	} else {
		consumerName = fmt.Sprintf("c-%d", os.Getpid())
	}
	// 重试参数默认值保护
	if retry.Base <= 0 {
		retry.Base = time.Second
	}
	if retry.Factor <= 0 {
		retry.Factor = 2.0
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Addr, Username: cfg.Username, Password: cfg.Password, DB: cfg.DB})
	ad := &redisAdapter{cfg: cfg, retry: retry, logger: logger, rdb: rdb, consumerName: consumerName, stopCh: make(chan struct{})}
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
				if err != nil {
					continue
				}
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
	if group == "" {
		group = "default"
	}
	// 确保 group 存在，使用 "0" 从头开始读取
	_ = r.rdb.XGroupCreateMkStream(ctx, topic, group, "0").Err()
	final := handler
	for i := len(mws) - 1; i >= 0; i-- {
		final = mws[i](final)
	}

	done := make(chan struct{})
	cctx, cancel := context.WithCancel(ctx)
	r.wg.Add(1)
	go func() {
		defer func() { r.wg.Done(); close(done) }()
		concurrency := r.cfg.ConsumerConcurrency
		if concurrency <= 0 {
			concurrency = 1
		}
		sem := make(chan struct{}, concurrency)
		for {
			select {
			case <-cctx.Done():
				return
			default:
			}
			// 2) 读取新消息（>）
			res, err := r.rdb.XReadGroup(cctx, &redis.XReadGroupArgs{
				Group:    group,
				Consumer: r.consumerName,
				Streams:  []string{topic, ">"},
				Count:    int64(concurrency),
				Block:    2 * time.Second,
			}).Result()
			if err == redis.Nil || (err != nil && cctx.Err() != nil) {
				continue
			}
			if err != nil {
				// 读取异常，短暂休眠避免紧循环
				time.Sleep(100 * time.Millisecond)
				continue
			}
			for _, str := range res {
				for _, xmsg := range str.Messages {
					sem <- struct{}{}
					go func(m redis.XMessage) {
						defer func() { <-sem }()
						msg := r.decodeXMessage(topic, m)
						if err := final(cctx, msg); err == nil {
							_, _ = r.rdb.XAck(cctx, topic, group, m.ID).Result()
						} else {
							// 失败：统一应用层重试（PublishDelay）+ 成功后 ACK；发布失败则不 ACK，待后续重试
							attempt := 0
							if s, ok := msg.Headers["x-retry-count"]; ok {
								if n, e := strconv.Atoi(s); e == nil {
									attempt = n
								}
							}
							nextAttempt := attempt + 1
							if nextAttempt <= r.retry.MaxRetries {
								h := copyHeaders(msg.Headers)
								h["x-retry-count"] = strconv.Itoa(nextAttempt)
								delay := time.Duration(float64(r.retry.Base) * math.Pow(r.retry.Factor, float64(nextAttempt-1)))
								if err := r.PublishDelay(cctx, Message{Topic: msg.Topic, Key: msg.Key, Body: msg.Body, Headers: h}, delay); err == nil {
									_, _ = r.rdb.XAck(cctx, topic, group, m.ID).Result()
									return
								}
							}
							// 超过最大重试或发布失败：不再重投（若未超出则因发布失败而不ACK以避免丢失）；超过最大重试则 ACK
							if nextAttempt > r.retry.MaxRetries {
								_, _ = r.rdb.XAck(cctx, topic, group, m.ID).Result()
							}
						}
					}(xmsg)
				}
			}
		}
	}()
	stop := func(sctx context.Context) error {
		cancel()
		select {
		case <-done:
			return nil
		case <-sctx.Done():
			return sctx.Err()
		}
	}
	return stop, nil
}

func (r *redisAdapter) Close(ctx context.Context) error {
	close(r.stopCh)
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
	default:
	}
	return r.rdb.Close()
}

func (r *redisAdapter) publishStream(ctx context.Context, msg Message) error {
	fields := map[string]interface{}{"key": msg.Key, "body": base64.StdEncoding.EncodeToString(msg.Body)}
	for k, v := range msg.Headers {
		fields["h:"+k] = v
	}
	return r.rdb.XAdd(ctx, &redis.XAddArgs{Stream: msg.Topic, Values: fields}).Err()
}

func (r *redisAdapter) decodeXMessage(topic string, xm redis.XMessage) Message {
	var key string
	var body []byte
	headers := make(map[string]string)
	for k, v := range xm.Values {
		switch k {
		case "key":
			key, _ = v.(string)
		case "body":
			if s, ok := v.(string); ok {
				body, _ = base64.StdEncoding.DecodeString(s)
			}
		default:
			if len(k) > 2 && k[:2] == "h:" {
				if s, ok := v.(string); ok {
					headers[k[2:]] = s
				}
			}
		}
	}
	return Message{Topic: topic, Key: key, Body: body, Headers: headers}
}
