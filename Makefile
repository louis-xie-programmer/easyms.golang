# EasyMS Makefile

# Docker 相关变量
DOCKER_COMPOSE := docker-compose
DOCKER_DIR := deploy/docker
DB_URL := postgresql://easy:easypass@172.29.16.1:5432/easydb?sslmode=disable

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

run-gateway:
	@echo "启动网关服务..."
	go run ./internal/platform/gateway/main

run-auth:
	@echo "启动认证服务..."
	go run ./internal/services/auth/cmd/authsvc

# 清理构建产物
clean:
	@echo "清理构建产物..."
	go clean
	rm -rf bin/
	rm -f logs/*

# 格式化代码
fmt:
	@echo "格式化代码..."
	go fmt ./...

# 迁移数据库
migrate:
	go run ./internal/platform/migrate -dbURL $(DB_URL) auto

push-config:
	@echo "推送配置..."
	go run ./internal/platform/push-config

# 检查代码问题
vet:
	@echo "检查代码问题..."
	go vet ./...