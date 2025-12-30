package taskbus

import (
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		name      string
		backoff   ExponentialBackoff
		attempt   int
		wantDelay time.Duration
		wantOK    bool
	}{
		{
			name:      "first attempt",
			backoff:   ExponentialBackoff{Base: time.Second, Factor: 2, MaxRetries: 3},
			attempt:   0,
			wantDelay: time.Second,
			wantOK:    true,
		},
		{
			name:      "second attempt",
			backoff:   ExponentialBackoff{Base: time.Second, Factor: 2, MaxRetries: 3},
			attempt:   1,
			wantDelay: 2 * time.Second,
			wantOK:    true,
		},
		{
			name:      "third attempt",
			backoff:   ExponentialBackoff{Base: time.Second, Factor: 2, MaxRetries: 3},
			attempt:   2,
			wantDelay: 4 * time.Second,
			wantOK:    true,
		},
		{
			name:      "max retries exceeded",
			backoff:   ExponentialBackoff{Base: time.Second, Factor: 2, MaxRetries: 3},
			attempt:   3,
			wantDelay: 0,
			wantOK:    false,
		},
		{
			name:      "custom factor",
			backoff:   ExponentialBackoff{Base: 100 * time.Millisecond, Factor: 3, MaxRetries: 5},
			attempt:   2,
			wantDelay: 900 * time.Millisecond,
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay, ok := tt.backoff.NextBackoff(tt.attempt)
			if ok != tt.wantOK {
				t.Errorf("NextBackoff() ok = %v, want %v", ok, tt.wantOK)
			}
			if delay != tt.wantDelay {
				t.Errorf("NextBackoff() delay = %v, want %v", delay, tt.wantDelay)
			}
		})
	}
}

func TestCopyHeaders(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]string
	}{
		{
			name:  "nil map",
			input: nil,
		},
		{
			name:  "empty map",
			input: map[string]string{},
		},
		{
			name:  "non-empty map",
			input: map[string]string{"key1": "value1", "key2": "value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := copyHeaders(tt.input)

			// 结果不应为 nil
			if result == nil {
				t.Fatal("copyHeaders() returned nil")
			}

			// 长度应匹配
			if len(result) != len(tt.input) {
				t.Errorf("copyHeaders() len = %d, want %d", len(result), len(tt.input))
			}

			// 值应匹配
			for k, v := range tt.input {
				if result[k] != v {
					t.Errorf("copyHeaders()[%s] = %s, want %s", k, result[k], v)
				}
			}

			// 应该是深拷贝
			if tt.input != nil && len(tt.input) > 0 {
				for k := range tt.input {
					result[k] = "modified"
					if tt.input[k] == "modified" {
						t.Error("copyHeaders() did not create a deep copy")
					}
					break
				}
			}
		})
	}
}

func TestMessage(t *testing.T) {
	msg := Message{
		Topic:   "test.topic",
		Key:     "test-key",
		Body:    []byte("test body"),
		Headers: map[string]string{"x-custom": "value"},
	}

	if msg.Topic != "test.topic" {
		t.Errorf("Message.Topic = %s, want test.topic", msg.Topic)
	}
	if msg.Key != "test-key" {
		t.Errorf("Message.Key = %s, want test-key", msg.Key)
	}
	if string(msg.Body) != "test body" {
		t.Errorf("Message.Body = %s, want 'test body'", string(msg.Body))
	}
	if msg.Headers["x-custom"] != "value" {
		t.Errorf("Message.Headers[x-custom] = %s, want value", msg.Headers["x-custom"])
	}
}

func TestEvent(t *testing.T) {
	event := Event{
		Topic:    "user.created",
		Type:     "UserCreated",
		Subject:  "user-123",
		Metadata: map[string]string{"source": "api"},
		Payload:  []byte(`{"name": "Alice"}`),
	}

	if event.Topic != "user.created" {
		t.Errorf("Event.Topic = %s, want user.created", event.Topic)
	}
	if event.Type != "UserCreated" {
		t.Errorf("Event.Type = %s, want UserCreated", event.Type)
	}
	if event.Subject != "user-123" {
		t.Errorf("Event.Subject = %s, want user-123", event.Subject)
	}
}

func TestFilterByType(t *testing.T) {
	filter := FilterByType("UserCreated")

	tests := []struct {
		name  string
		event Event
		want  bool
	}{
		{
			name:  "matching type",
			event: Event{Type: "UserCreated"},
			want:  true,
		},
		{
			name:  "non-matching type",
			event: Event{Type: "UserDeleted"},
			want:  false,
		},
		{
			name:  "empty type",
			event: Event{Type: ""},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filter(tt.event); got != tt.want {
				t.Errorf("FilterByType()(%v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestEnqueueOptions(t *testing.T) {
	t.Run("WithDelay", func(t *testing.T) {
		opts := &enqueueOpts{}
		WithDelay(5 * time.Second)(opts)
		if opts.delay != 5*time.Second {
			t.Errorf("WithDelay() delay = %v, want 5s", opts.delay)
		}
	})

	t.Run("WithKey", func(t *testing.T) {
		opts := &enqueueOpts{}
		WithKey("my-key")(opts)
		if opts.key != "my-key" {
			t.Errorf("WithKey() key = %s, want my-key", opts.key)
		}
	})

	t.Run("combined options", func(t *testing.T) {
		opts := &enqueueOpts{}
		WithDelay(10 * time.Second)(opts)
		WithKey("combined-key")(opts)
		if opts.delay != 10*time.Second {
			t.Errorf("delay = %v, want 10s", opts.delay)
		}
		if opts.key != "combined-key" {
			t.Errorf("key = %s, want combined-key", opts.key)
		}
	})
}
