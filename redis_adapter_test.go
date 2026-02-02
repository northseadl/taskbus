package taskbus

import "testing"

func TestRedisStreamForTopic(t *testing.T) {
	tests := []struct {
		name  string
		topic string
		want  string
	}{
		{name: "job topic", topic: "taskbus.myns.job.email.send", want: "taskbus.myns.job"},
		{name: "event topic", topic: "taskbus.myns.event.user.created", want: "taskbus.myns.event"},
		{name: "cron topic", topic: "taskbus.myns.cron.cleanup", want: "taskbus.myns.cron"},
		{name: "global event topic", topic: "taskbus.event.order.paid", want: "taskbus.event"},
		{name: "non-taskbus topic", topic: "custom.topic", want: "custom.topic"},
		{name: "unknown component", topic: "taskbus.ns.unknown.topic", want: "taskbus.ns.unknown.topic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redisStreamForTopic(tt.topic)
			if got != tt.want {
				t.Errorf("redisStreamForTopic(%q) = %q, want %q", tt.topic, got, tt.want)
			}
		})
	}
}
