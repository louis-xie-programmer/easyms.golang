# EasyMS Makefile

.PHONY: test test-cover build clean

# 运行所有测试
test:
	@echo "运行所有测试..."
	ENV=test go test -v ./...

# 运行测试并生成覆盖率报告
test-cover:
	@echo "运行测试并生成覆盖率报告..."
	ENV=test go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "HTML格式的覆盖率报告已生成: coverage.html"

# 构建所有服务
build:
	@echo "构建所有服务..."
	docker-compose build

# 启动所有服务
run:
	@echo "启动所有服务..."
	docker-compose up -d

# 停止所有服务
stop:
	@echo "停止所有服务..."
	docker-compose down

# 清理构建产物
clean:
	@echo "清理构建产物..."
	go clean
	rm -f coverage.out
	rm -f coverage.html

# 格式化代码
fmt:
	@echo "格式化代码..."
	go fmt ./...

# 检查代码问题
vet:
	@echo "检查代码问题..."
	go vet ./...