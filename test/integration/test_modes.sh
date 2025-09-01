#!/bin/bash

# TaskBus 集成测试模块 - 多模式测试脚本

set -e

echo "=== TaskBus 集成测试模块化支持演示 ==="
echo

# 函数：显示当前模块状态
show_module_status() {
    echo "当前模块状态："
    echo "- 模块名称: $(go list -m)"
    echo "- TaskBus 版本: $(go list -m github.com/northseadl/taskbus)"
    echo
}

# 模式 1：本地开发模式（使用 replace）
echo "🔧 模式 1: 本地开发模式"
echo "使用本地 TaskBus 代码进行测试..."
go mod edit -replace=github.com/northseadl/taskbus=../..
go mod tidy
show_module_status

echo "运行快速测试（无 Docker 环境）："
go test -v -run TestRabbitMQ_EndToEnd
echo

# 模式 2：发布版本测试模式
echo "📦 模式 2: 发布版本测试模式"
echo "移除本地替换，使用已发布版本..."
go mod edit -dropreplace=github.com/northseadl/taskbus
go mod tidy
show_module_status

echo "运行快速测试（无 Docker 环境）："
go test -v -run TestRabbitMQ_EndToEnd
echo

# 模式 3：特定版本测试模式
echo "🎯 模式 3: 特定版本测试模式"
echo "测试特定版本的兼容性..."

# 注意：这里使用 v0.2.0 作为示例，实际使用时可以指定任何已发布的版本
echo "指定版本为 v0.2.0..."
go mod edit -require=github.com/northseadl/taskbus@v0.2.0
go mod tidy
show_module_status

echo "运行快速测试（无 Docker 环境）："
go test -v -run TestRabbitMQ_EndToEnd
echo

# 恢复到本地开发模式
echo "🔄 恢复到本地开发模式"
go mod edit -replace=github.com/northseadl/taskbus=../..
go mod tidy
show_module_status

echo "✅ 模块化测试演示完成！"
echo
echo "💡 提示："
echo "- 本地开发: go mod edit -replace=github.com/northseadl/taskbus=../.."
echo "- 发布版本: go mod edit -dropreplace=github.com/northseadl/taskbus"
echo "- 特定版本: go mod edit -require=github.com/northseadl/taskbus@vX.Y.Z"
echo "- 完整测试: ./run_integration.sh"
