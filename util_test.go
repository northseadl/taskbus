package taskbus

import "testing"

func TestTrimTopicPrefix(t *testing.T) {
	tests := []struct {
		name   string
		topic  string
		prefix string
		want   string
	}{
		{
			name:   "matching prefix",
			topic:  "taskbus.myns.job.example",
			prefix: "taskbus.myns.job.",
			want:   "example",
		},
		{
			name:   "no matching prefix",
			topic:  "other.topic.name",
			prefix: "taskbus.myns.job.",
			want:   "other.topic.name",
		},
		{
			name:   "empty topic",
			topic:  "",
			prefix: "taskbus.",
			want:   "",
		},
		{
			name:   "empty prefix",
			topic:  "some.topic",
			prefix: "",
			want:   "some.topic",
		},
		{
			name:   "prefix longer than topic",
			topic:  "short",
			prefix: "very.long.prefix.",
			want:   "short",
		},
		{
			name:   "exact match",
			topic:  "taskbus.myns.job.",
			prefix: "taskbus.myns.job.",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimTopicPrefix(tt.topic, tt.prefix)
			if got != tt.want {
				t.Errorf("trimTopicPrefix(%q, %q) = %q, want %q", tt.topic, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestBuildTopicPrefix(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		component string
		want      string
	}{
		{
			name:      "job component",
			namespace: "myns",
			component: "job",
			want:      "taskbus.myns.job.",
		},
		{
			name:      "event component",
			namespace: "my-service",
			component: "event",
			want:      "taskbus.my-service.event.",
		},
		{
			name:      "cron component",
			namespace: "app",
			component: "cron",
			want:      "taskbus.app.cron.",
		},
		{
			name:      "empty namespace",
			namespace: "",
			component: "job",
			want:      "taskbus..job.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTopicPrefix(tt.namespace, tt.component)
			if got != tt.want {
				t.Errorf("buildTopicPrefix(%q, %q) = %q, want %q", tt.namespace, tt.component, got, tt.want)
			}
		})
	}
}

func TestBuildTopic(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		component string
		topicName string
		want      string
	}{
		{
			name:      "job topic",
			namespace: "myns",
			component: "job",
			topicName: "email.send",
			want:      "taskbus.myns.job.email.send",
		},
		{
			name:      "event topic",
			namespace: "service",
			component: "event",
			topicName: "user.created",
			want:      "taskbus.service.event.user.created",
		},
		{
			name:      "cron topic",
			namespace: "app",
			component: "cron",
			topicName: "cleanup",
			want:      "taskbus.app.cron.cleanup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTopic(tt.namespace, tt.component, tt.topicName)
			if got != tt.want {
				t.Errorf("buildTopic(%q, %q, %q) = %q, want %q",
					tt.namespace, tt.component, tt.topicName, got, tt.want)
			}
		})
	}
}

func TestBuildWildcardTopic(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		component string
		want      string
	}{
		{
			name:      "job wildcard",
			namespace: "myns",
			component: "job",
			want:      "taskbus.myns.job.#",
		},
		{
			name:      "event wildcard",
			namespace: "service",
			component: "event",
			want:      "taskbus.service.event.#",
		},
		{
			name:      "cron wildcard",
			namespace: "app",
			component: "cron",
			want:      "taskbus.app.cron.#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildWildcardTopic(tt.namespace, tt.component)
			if got != tt.want {
				t.Errorf("buildWildcardTopic(%q, %q) = %q, want %q",
					tt.namespace, tt.component, got, tt.want)
			}
		})
	}
}

func TestMatchTopic(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		topic   string
		want    bool
	}{
		{name: "exact match", pattern: "a.b.c", topic: "a.b.c", want: true},
		{name: "exact mismatch", pattern: "a.b.c", topic: "a.b.d", want: false},
		{name: "star matches one", pattern: "a.*.c", topic: "a.b.c", want: true},
		{name: "star does not match empty", pattern: "a.*.c", topic: "a.c", want: false},
		{name: "hash matches many", pattern: "a.#", topic: "a.b.c", want: true},
		{name: "hash matches zero", pattern: "a.#", topic: "a", want: true},
		{name: "hash middle", pattern: "a.#.c", topic: "a.b.c", want: true},
		{name: "hash middle mismatch", pattern: "a.#.c", topic: "a.b.d", want: false},
		{name: "hash only", pattern: "#", topic: "a.b.c", want: true},
		{name: "empty pattern", pattern: "", topic: "a", want: false},
		{name: "empty topic", pattern: "a", topic: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTopic(tt.pattern, tt.topic)
			if got != tt.want {
				t.Errorf("matchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, got, tt.want)
			}
		})
	}
}
