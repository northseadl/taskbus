package taskbus

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockKV 是 KV 接口的内存实现，用于测试。
type mockKV struct {
	mu   sync.Mutex
	data map[string]time.Time // key -> 过期时间
}

func newMockKV() *mockKV {
	return &mockKV{data: make(map[string]time.Time)}
}

func (m *mockKV) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在且未过期
	if expiry, ok := m.data[key]; ok && time.Now().Before(expiry) {
		return false, nil
	}

	m.data[key] = time.Now().Add(ttl)
	return true, nil
}

func TestIdempotencyMiddleware_Basic(t *testing.T) {
	kv := newMockKV()
	cfg := IdempotencyConfig{
		KV:     kv,
		Prefix: "test:idem",
		TTL:    time.Hour,
	}

	mw := NewIdempotencyMiddleware(cfg)

	// 记录执行次数
	execCount := 0
	handler := func(ctx context.Context, msg Message) error {
		execCount++
		return nil
	}

	wrapped := mw(handler)
	ctx := context.Background()
	msg := Message{Key: "unique-key-1", Body: []byte("test")}

	// 第一次执行应该成功
	err := wrapped(ctx, msg)
	if err != nil {
		t.Fatalf("First execution error: %v", err)
	}
	if execCount != 1 {
		t.Errorf("execCount = %d, want 1", execCount)
	}

	// 第二次执行相同 key 应该被跳过
	err = wrapped(ctx, msg)
	if err != nil {
		t.Fatalf("Second execution error: %v", err)
	}
	if execCount != 1 {
		t.Errorf("execCount = %d after second call, want 1 (should be skipped)", execCount)
	}
}

func TestIdempotencyMiddleware_DifferentKeys(t *testing.T) {
	kv := newMockKV()
	cfg := IdempotencyConfig{
		KV:     kv,
		Prefix: "test:idem",
		TTL:    time.Hour,
	}

	mw := NewIdempotencyMiddleware(cfg)
	execCount := 0
	handler := func(ctx context.Context, msg Message) error {
		execCount++
		return nil
	}

	wrapped := mw(handler)
	ctx := context.Background()

	// 不同的 key 应该都执行
	_ = wrapped(ctx, Message{Key: "key-1"})
	_ = wrapped(ctx, Message{Key: "key-2"})
	_ = wrapped(ctx, Message{Key: "key-3"})

	if execCount != 3 {
		t.Errorf("execCount = %d, want 3 (different keys)", execCount)
	}
}

func TestIdempotencyMiddleware_EmptyKey(t *testing.T) {
	kv := newMockKV()
	cfg := IdempotencyConfig{
		KV:     kv,
		Prefix: "test:idem",
		TTL:    time.Hour,
	}

	mw := NewIdempotencyMiddleware(cfg)
	execCount := 0
	handler := func(ctx context.Context, msg Message) error {
		execCount++
		return nil
	}

	wrapped := mw(handler)
	ctx := context.Background()

	// 空 key 应该直接执行（不走幂等检查）
	_ = wrapped(ctx, Message{Key: ""})
	_ = wrapped(ctx, Message{Key: ""})

	if execCount != 2 {
		t.Errorf("execCount = %d, want 2 (empty key bypasses idempotency)", execCount)
	}
}

func TestIdempotencyMiddleware_WithKeyFunc(t *testing.T) {
	kv := newMockKV()
	cfg := IdempotencyConfig{
		KV:     kv,
		Prefix: "test:idem",
		TTL:    time.Hour,
		KeyFunc: func(ctx context.Context, m Message) (string, error) {
			return string(m.Body), nil // 使用 body 作为 key
		},
	}

	mw := NewIdempotencyMiddleware(cfg)
	execCount := 0
	handler := func(ctx context.Context, msg Message) error {
		execCount++
		return nil
	}

	wrapped := mw(handler)
	ctx := context.Background()

	// 相同 body 应该只执行一次
	_ = wrapped(ctx, Message{Body: []byte("same-body")})
	_ = wrapped(ctx, Message{Body: []byte("same-body")})

	if execCount != 1 {
		t.Errorf("execCount = %d, want 1 (same body)", execCount)
	}

	// 不同 body 应该执行
	_ = wrapped(ctx, Message{Body: []byte("different-body")})

	if execCount != 2 {
		t.Errorf("execCount = %d, want 2 (different body)", execCount)
	}
}

func TestIdempotencyMiddleware_DefaultPrefix(t *testing.T) {
	kv := newMockKV()
	cfg := IdempotencyConfig{
		KV:  kv,
		TTL: time.Hour,
		// Prefix 为空，应该使用默认值 "tq:idem"
	}

	// 不应该 panic
	mw := NewIdempotencyMiddleware(cfg)
	if mw == nil {
		t.Error("NewIdempotencyMiddleware returned nil")
	}
}

func TestIdempotencyMiddleware_DefaultTTL(t *testing.T) {
	kv := newMockKV()
	cfg := IdempotencyConfig{
		KV: kv,
		// TTL 为 0，应该使用默认值 24 小时
	}

	mw := NewIdempotencyMiddleware(cfg)
	if mw == nil {
		t.Error("NewIdempotencyMiddleware returned nil")
	}
}

func TestIdempotencyMiddleware_NilKV_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewIdempotencyMiddleware with nil KV should panic")
		}
	}()

	cfg := IdempotencyConfig{
		KV: nil, // 应该触发 panic
	}
	_ = NewIdempotencyMiddleware(cfg)
}

func TestJobIdempotencyMiddleware(t *testing.T) {
	kv := newMockKV()
	cfg := IdempotencyConfig{
		KV:     kv,
		Prefix: "test:job:idem",
		TTL:    time.Hour,
		KeyFunc: func(ctx context.Context, m Message) (string, error) {
			return string(m.Body), nil
		},
	}

	mw := NewJobIdempotencyMiddleware(cfg)
	execCount := 0
	handler := func(ctx context.Context, jobName string, payload []byte) error {
		execCount++
		return nil
	}

	wrapped := mw(handler)
	ctx := context.Background()

	// 相同 payload 应该只执行一次
	_ = wrapped(ctx, "test-job", []byte("same-payload"))
	_ = wrapped(ctx, "test-job", []byte("same-payload"))

	if execCount != 1 {
		t.Errorf("execCount = %d, want 1", execCount)
	}

	// 不同 payload 应该执行
	_ = wrapped(ctx, "test-job", []byte("different-payload"))

	if execCount != 2 {
		t.Errorf("execCount = %d, want 2", execCount)
	}
}

func TestJobIdempotencyMiddleware_ContextKey(t *testing.T) {
	kv := newMockKV()
	cfg := IdempotencyConfig{
		KV:     kv,
		Prefix: "test:job:idem",
		TTL:    time.Hour,
		KeyFunc: func(ctx context.Context, m Message) (string, error) {
			return m.Key, nil
		},
	}

	mw := NewJobIdempotencyMiddleware(cfg)
	execCount := 0
	handler := func(ctx context.Context, jobName string, payload []byte) error {
		execCount++
		return nil
	}

	wrapped := mw(handler)
	ctx := withJobKey(context.Background(), "job-key-1")

	_ = wrapped(ctx, "test-job", []byte("payload-1"))
	_ = wrapped(ctx, "test-job", []byte("payload-2"))

	if execCount != 1 {
		t.Errorf("execCount = %d, want 1 (same context key)", execCount)
	}
}

func TestRedisKV(t *testing.T) {
	// RedisKV 结构体测试（不依赖真实 Redis）
	// 这里只测试结构是否正确实现了 KV 接口
	var _ KV = RedisKV{}
}
