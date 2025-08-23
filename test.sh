#!/bin/bash

# EasyMS 测试脚本

echo "开始执行 EasyMS 测试..."

# 设置测试环境变量
export ENV=test

# 运行所有测试
echo "运行所有测试..."
go test -v ./...

# 检查测试结果
if [ $? -eq 0 ]; then
    echo "所有测试通过!"
else
    echo "测试失败!"
    exit 1
fi

# 运行覆盖率测试
echo "运行测试覆盖率分析..."
go test -coverprofile=coverage.out ./...

# 检查覆盖率结果
if [ $? -eq 0 ]; then
    echo "测试覆盖率报告已生成: coverage.out"
    go tool cover -html=coverage.out -o coverage.html
    echo "HTML格式的覆盖率报告已生成: coverage.html"
else
    echo "覆盖率测试失败!"
    exit 1
fi

echo "测试完成!"