package main

import (
	"context"
	"fmt"
	"time"

	taskbus "github.com/northseadl/taskbus"
)

func main() {
	ctx := context.Background()

	// 使用本地 Cron（无需外部依赖），每秒打印一次，运行约 3.5 秒后退出
	cfg := taskbus.Config{Cron: taskbus.CronConfig{Distributed: false}}
	cli, err := taskbus.New(ctx, cfg)
	if err != nil { panic(err) }
	defer func() { _ = cli.Close(ctx) }()

	count := 0
	_, err = cli.Cron().Add("*/1 * * * * *", "tick-1s", func(ctx context.Context) error {
		count++
		fmt.Println("[Cron] tick", count)
		return nil
	})
	if err != nil { panic(err) }

	_ = cli.Cron().Start(ctx)
	fmt.Println("[Cron] 已启动，本示例将运行约 3.5s...")
	time.Sleep(3500 * time.Millisecond)
	_ = cli.Cron().Stop(ctx)
	fmt.Println("[Cron] 已停止，示例结束")
}

