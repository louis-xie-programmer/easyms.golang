# EasyMS Makefile

# Docker 相关变量
DOCKER_COMPOSE := docker-compose
DOCKER_DIR := deploy/docker

.PHONY: build run stop clean fmt vet

# 构建所有服务
build:
	@echo "构建所有服务..."
	cd $(DOCKER_DIR) && $(DOCKER_COMPOSE) -f docker-compose.yaml build

# 启动所有服务
run:
	@echo "启动所有服务..."
	cd $(DOCKER_DIR) && $(DOCKER_COMPOSE) -f docker-compose.yaml up -d

# 停止所有服务
stop:
	@echo "停止所有服务..."
	cd $(DOCKER_DIR) && $(DOCKER_COMPOSE) -f docker-compose.yaml down

# 清理构建产物
clean:
	@echo "清理构建产物..."
	go clean
	rm -rf bin/

# 格式化代码
fmt:
	@echo "格式化代码..."
	go fmt ./...

# 检查代码问题
vet:
	@echo "检查代码问题..."
	go vet ./...