package taskbus

import (
	"context"
	"log"
)

// Logger 为最小日志接口，应用可注入自定义实现。
type Logger interface {
	Info(ctx context.Context, msg string, kv ...interface{})
	Error(ctx context.Context, msg string, kv ...interface{})
}

// defaultLogger 使用标准库 log，满足最小可用。
type defaultLogger struct{}

func (defaultLogger) Info(ctx context.Context, msg string, kv ...interface{}) {
	log.Println(append([]interface{}{"INFO", msg}, kv...)...)
}
func (defaultLogger) Error(ctx context.Context, msg string, kv ...interface{}) {
	log.Println(append([]interface{}{"ERROR", msg}, kv...)...)
}
