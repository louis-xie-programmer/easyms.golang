# EasyMS - Golang 微服务架构实践框架

> **项目代号：easyms.golang**  
> **项目定位：生产可用、模块化、工程治理完备的 Golang 微服务架构实践与样例项目**  
> **目标受众：后端架构师、Golang 工程师、DevOps 实践者、团队技术负责人**

[![Go Report Card](https://goreportcard.com/badge/github.com/yourusername/easyms)](https://goreportcard.com/report/github.com/yourusername/easyms)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

---
以下是当前相关的博文，结合博文，可快速了解本系统的核心功能和实现原理, 后续会持续更新，欢迎大家扫码关注"代码扳手"公众号，获取更多技术交流信息。

![wx.jpg](wx.jpg)
## 一、项目背景：微服务的现实挑战

在当前云原生基础设施和中大型系统架构设计日趋成熟的背景下，微服务已不再是"是否采用"的问题，而是"如何落地"的问题。

然而，在 Golang 技术栈中，虽然具备构建高性能微服务的语言优势，却缺少一套 **系统化、可复用、符合现代软件工程要求的微服务架构实践模板**。目前存在的问题包括：

- 项目结构松散，缺少清晰的分层和职责边界；
- 微服务基础能力（注册发现、配置、追踪、日志、监控、权限等）各自为政，缺少统一治理；
- 中间件接入重复繁琐，缺乏抽象和封装；
- DevOps 流程不规范，测试部署体系割裂，环境切换复杂；
- 教学项目偏多，真正可落地、可迭代的工程项目稀缺。

> **easyms.golang 的使命是：构建一个开源的、工程化的、完整的 Golang 微服务体系，实现架构治理、开发效率与可观测性的统一。**

---

## 二、项目目录结构

```
.
├── configs/                    # 配置文件
│   ├── app.yaml               # 应用基本配置
│   ├── share/                 # 共享配置
│   │   ├── dev.yaml           # 开发环境共享配置
│   │   └── prod.yaml          # 生产环境共享配置
│   ├── auth-svc/              # 认证服务配置
│   │   ├── dev.yaml           # 开发环境配置
│   │   └── prod.yaml          # 生产环境配置
│   ├── gateway/               # 网关服务配置
│   │   ├── dev.yaml           # 开发环境配置
│   │   └── routes.yaml        # 路由配置
│   └── user-svc/              # 用户服务配置
│       ├── dev.yaml           # 开发环境配置
│       └── prod.yaml          # 生产环境配置
├── deploy/                    # 部署相关文件
│   └── docker/                # Docker部署文件
│       ├── docker-compose.yaml # Docker Compose配置
│       └── promtail/          # Promtail配置
├── internal/                  # 内部包（不允许外部导入）
│   ├── platform/              # 平台级服务
│   │   ├── gateway/           # API网关
│   │   │   ├── main/          # 网关服务入口
│   │   │   ├── Dockerfile     # 网关Dockerfile
│   │   │   └── gateway.go     # 网关核心实现
│   │   ├── migrate/           # 数据库迁移工具
│   │   │   └── main.go        # 迁移工具入口
│   │   └── push-config/       # 配置推送工具
│   │       └── main.go        # 配置推送入口
│   ├── services/              # 业务服务
│   │   ├── auth/              # 认证服务
│   │   │   ├── cmd/           # 服务入口
│   │   │   │   └── authsvc/   # 认证服务主程序
│   │   │   │       └── main.go # 认证服务入口
│   │   │   ├── configs/       # 认证服务配置
│   │   │   └── internal/      # 服务私有代码
│   │   │       ├── consts/    # 常量定义
│   │   │       ├── handles/   # 处理函数
│   │   │       ├── middleware/ # 中间件
│   │   │       ├── service/   # 业务逻辑
│   │   │       └── storage/   # 数据存储
│   │   └── user/              # 用户服务
│   │       ├── cmd/           # 服务入口
│   │       │   └── usersvc/   # 用户服务主程序
│   │       │       └── main.go # 用户服务入口
│   │       ├── configs/       # 用户服务配置
│   │       └── internal/      # 服务私有代码
│   └── shared/                # 共享组件
│       ├── config/            # 配置管理
│       ├── db/                # 数据库访问
│       ├── discovery/         # 服务发现
│       ├── entities/          # 实体定义
│       ├── logger/            # 日志系统
│       ├── middleware/        # 中间件
│       └── models/            # 数据模型
├── migrations/                # 数据库迁移脚本
└── Makefile                   # 构建脚本
```


## 二、架构总览

### 1. 设计原则

- **单一职责**：每个模块仅负责一项核心功能，降低耦合度
- **开闭原则**：对扩展开放，对修改封闭，支持插件化集成
- **依赖倒置**：高层模块不依赖低层模块，两者都依赖抽象
- **接口隔离**：使用最小接口，避免不必要的依赖
- **无状态设计**：服务实例间无状态共享，支持水平扩展

### 2. 配置管理规范

采用统一配置源管理策略，支持 Consul 和本地文件两种模式：

```yaml
# configs/app.yaml
env: dev
store_type: consul # 或 local
consul:
  host: localhost:8500
  key_path: easyms/%s/%s.yaml
  reload_on_changes: true  # 启用配置热更新
```

**配置加载规则**：
1. 配置源必须严格二选一：全部使用 Consul 或全部使用本地配置
2. Consul 模式下所有配置从 KV 存储获取，路径规范：
   - 共享配置：`easyms/share/{env}.yaml`
   - 私有配置：`easyms/{service}/{env}.yaml`
3. 本地模式配置路径：
   - 共享配置：`configs/share/{env}.yaml`
   - 私有配置：`configs/{service}/{env}.yaml`
4. 配置合并使用深度合并算法，私有配置优先级高于共享配置

### 3. 日志规范

``go
// internal/shared/logger 实现
logger.Info().Str("service", "server1").Msg("Service started")
```

- 结构化 JSON 格式输出
- 支持多级管道分流（控制台/文件/Loki）
- 动态日志级别配置

---

## 三、核心模块详解

### 1. 服务注册与发现

基于 Consul 实现服务注册与发现，支持健康检查和服务元数据。

### 2. 配置管理与热更新

系统支持两种配置管理模式：本地文件和 Consul 配置中心，并提供配置热更新机制。

**配置热更新特性：**
1. **实时推送机制**：当 Consul 中的配置发生变化时，服务能实时感知并加载新配置
2. **动态调整参数**：支持服务运行时动态调整配置参数，无需重启服务
3. **配置版本管理**：提供配置版本管理和回滚功能，确保配置变更的安全性

**配置管理API接口：**
- `POST /config/version` - 保存当前配置为新版本
- `GET /config/versions` - 获取配置版本列表
- `GET /config/version/:versionID` - 获取指定版本的配置详情
- `POST /config/rollback/:versionID` - 回滚到指定版本
- `GET /config/current` - 获取当前配置
- `PUT /config/current` - 更新当前配置

### 3. API 网关

内置反向代理和负载均衡功能，支持服务路由、熔断和限流。

### 4. 熔断器

集成 go-circuitbreaker 实现服务熔断机制，防止故障扩散。

### 5. 限流器

支持 IP 和 User-Agent 维度的动态限流，规则可通过 Consul 或 YAML 配置，热更新秒级生效。

### 6. 日志系统

结构化日志记录，支持 Loki 日志收集，提供统一的日志查询和分析能力。

### 7. 认证授权

内置 OAuth2 认证服务，支持密码模式和刷新令牌模式。

### 8. 可观测性

链路追踪、指标监控等可观测性能力，便于系统运维和问题排查。

### 9. 数据层优化

数据库连接池管理、读写分离支持、数据库迁移工具集成，提升数据访问性能。

### 10. 缓存防护

Redis 缓存穿透、击穿、雪崩防护机制，保障系统稳定性。

## 四、技术选型

| 组件          | 版本       | 用途                |
|---------------|------------|---------------------|
| Golang        | 1.24.0+    | 核心开发语言        |
| Consul        | 1.32.1     | 服务注册与配置中心   |
| zerolog       | 1.34.0     | 结构化日志库        |
| Loki          | latest     | 日志聚合系统        |
| Promtail      | latest     | 日志收集代理        |
| gopkg.in/yaml | v2.4.0     | YAML 解析库         |

---

## 五、网关熔断与动态限流：高可用微服务的护城河

### 1. 功能亮点

- **熔断保护**：基于 [mercari/go-circuitbreaker](https://github.com/mercari/go-circuitbreaker)，为每个后端实例独立熔断，自动隔离异常节点，防止雪崩。
- **多元限流**：支持按 IP 段、UserAgent（正则）等多维度限流，规则可通过 Consul 或 YAML 配置，热更新秒级生效。
- **配置热更新**：限流与熔断参数均可动态调整，无需重启服务，适应突发流量和业务变化。
- **可观测性**：熔断状态变更、限流命中均可埋点，便于监控与报警。

### 2. 原理与关键实现

#### 熔断器（Circuit Breaker）

- 每个后端实例分配独立熔断器，支持失败率、窗口、半开等参数灵活配置。
- 状态流转（Closed→Open→Half-Open→Closed）自动管理，Ready/Done 标准用法，防止误用。
- 状态变更支持回调，可集成日志与监控。

## 六、配置管理与热更新详解

### 1. 配置热更新机制

系统通过 Consul 的键值存储实现配置热更新机制：

1. **配置监听**：服务启动时根据 `reload_on_changes` 配置决定是否启用配置监听
2. **变更检测**：配置监听器定期轮询 Consul 中的配置变化
3. **动态加载**：检测到配置变化后，自动重新加载配置并应用到运行中的服务
4. **安全回滚**：提供配置版本管理和回滚功能，确保配置变更的安全性

### 2. 配置版本管理

系统支持配置版本管理，包括：

- 自动保存配置变更历史
- 查询历史版本信息
- 回滚到任意历史版本

### 3. 使用示例

在 `configs/app.yaml` 中启用配置热更新：

```
consul:
  reload_on_changes: true
```

通过 API 管理配置版本：

```bash
# 保存当前配置为新版本
curl -X POST http://localhost:10001/config/version \
  -H "Content-Type: application/json" \
  -d '{"description":"Initial configuration"}'

# 查看配置版本列表
curl http://localhost:10001/config/versions

# 回滚到指定版本
curl -X POST http://localhost:10001/config/rollback/version-id-12345
```

## 七、构建和运行

### 1. 本地构建

```bash
# 构建网关服务
go build -o gateway ./internal/platform/gateway/main/main.go

# 构建认证服务
go build -o auth-svc ./internal/services/auth/cmd/authsvc/main.go

# 构建用户服务
go build -o user-svc ./internal/services/user/cmd/usersvc/main.go

# 构建迁移工具
go build -o migrate ./internal/platform/migrate/main.go
```

### 2. Docker构建

```bash
# 进入docker目录
cd deploy/docker

# 构建所有服务
docker-compose build

# 启动所有服务
docker-compose up -d
```

## 八、数据库迁移工具

项目集成了 `golang-migrate` 数据库迁移工具，用于管理数据库模式的演进。

### 1. 功能特点

- 支持向上迁移（应用新版本）和向下迁移（回滚）
- 使用 SQL 文件管理数据库模式变更
- 支持多种数据库类型（当前项目使用 PostgreSQL）

### 2. 使用方法

```bash
# 应用所有未执行的迁移
go run ./internal/platform/migrate/main.go -database "postgres://user:password@host:port/dbname?sslmode=disable" -up

# 回滚所有已执行的迁移
go run ./internal/platform/migrate/main.go -database "postgres://user:password@host:port/dbname?sslmode=disable" -down

# 使用 GORM 自动迁移（推荐用于开发环境）
go run ./internal/platform/migrate/main.go -database "postgres://user:password@host:port/dbname?sslmode=disable" -auto

# 指定迁移文件路径
go run ./internal/platform/migrate/main.go -path "./migrations" -database "postgres://user:password@host:port/dbname?sslmode=disable" -up
```

### 3. 迁移文件结构

迁移文件位于 `migrations/` 目录下，按照以下命名规范：

```
migrations/
├── 000001_create_users_table.up.sql    # 向上迁移脚本
├── 000001_create_users_table.down.sql  # 向下迁移脚本
├── 000002_add_email_to_users.up.sql
└── 000002_add_email_to_users.down.sql
```

每个迁移都有对应的 `.up.sql` 和 `.down.sql` 文件，分别用于应用和回滚迁移。

## 十、Docker 部署

项目提供了完整的 Docker Compose 部署方案，包括以下组件：

### 1. 服务组件

- **Consul**：服务注册与配置中心
- **PostgreSQL**：主数据库
- **Redis**：缓存系统
- **Loki**：日志聚合系统
- **Grafana**：可视化监控面板
- **Promtail**：日志收集代理
- **API Gateway**：API网关
- **Auth Service**：认证服务
- **User Service**：用户服务

### 2. 网络配置

所有服务都在同一个自定义网络 `easy-network` 中，确保服务间的通信安全。

### 3. 数据持久化

通过 Docker 卷（volumes）实现数据持久化：
- Consul 数据存储在 `./consul/data`
- PostgreSQL 数据存储在 `./postgres/data`
- Loki 数据存储在 `loki_data` 卷
- Grafana 数据存储在 `grafana_data` 卷

### 4. 部署命令

```bash
cd deploy/docker
docker-compose up -d
```

### 5. 访问地址

- **Consul UI**：http://localhost:8500
- **Grafana**：http://localhost:3000 (默认账号密码: admin/admin)
- **API Gateway**：http://localhost:10000
- **Auth Service**：http://localhost:10001
- **User Service**：http://localhost:10002

## 十一、Makefile 使用指南

项目提供了 Makefile 来简化常见的开发和部署任务。

### 1. 构建相关命令

```bash
# 构建所有服务的 Docker 镜像
make build

# 清理构建产物
make clean
```

### 2. 运行相关命令

```bash
# 启动所有服务
make run

# 停止所有服务
make stop

# 启动网关服务
make run-gateway

# 启动认证服务
make run-auth
```

### 3. 代码质量相关命令

```bash
# 格式化代码
make fmt

# 检查代码问题
make vet
```

### 4. 数据库迁移

```bash
# 执行数据库自动迁移（使用 GORM AutoMigrate）
make migrate
```

### 5. 其他命令

```bash
# 显示帮助信息
make help
```

## 十二、快速开始

### 1. 环境准备

确保安装以下依赖：

- Go 1.24+
- Docker & Docker Compose

### 2. 启动服务

```bash
# 使用 Docker Compose 启动所有服务
make run

# 或者手动启动
cd deploy/docker
docker-compose up -d
```

## 十三、技术栈

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | 1.25.2 | 核心开发语言 |
| Gin | v1.10.1 | Web 框架 |
| Consul | 1.32.1 | 服务发现与配置中心 |
| PostgreSQL | 18.1 | 主数据库 |
| Redis | 7-alpine | 缓存系统 |
| Loki | 2.9.4 | 日志收集系统 |
| go-circuitbreaker | 0.0.2 | 熔断器实现 |
| golang-jwt | v5.3.0 | JWT支持 |

## 十四、API 文档

### OAuth2 接口

- `POST /oauth2/token` - 获取访问令牌
- `POST /oauth2/check_token` - 检查令牌有效性
- `POST /oauth2/login` - 用户登录

### 配置管理接口

- `POST /config/version` - 保存配置版本
- `GET /config/versions` - 获取配置版本列表
- `GET /config/version/:versionID` - 获取指定版本详情
- `POST /config/rollback/:versionID` - 回滚到指定版本
- `GET /config/current` - 获取当前配置
- `PUT /config/current` - 更新当前配置

## 十五、核心特性详解

### 1. 服务注册与发现

基于 Consul 实现服务注册与发现，支持健康检查和服务元数据。

### 2. 配置管理与热更新

系统支持两种配置管理模式：本地文件和 Consul 配置中心，并提供配置热更新机制。

### 3. API 网关

内置反向代理和负载均衡功能，支持服务路由、熔断和限流。

### 4. 熔断器

集成 go-circuitbreaker 实现服务熔断机制，防止故障扩散。

### 5. 限流器

支持 IP 和 User-Agent 维度的动态限流，规则可通过 Consul 或 YAML 配置，热更新秒级生效。

### 6. 日志系统

结构化日志记录，支持 Loki 日志收集，提供统一的日志查询和分析能力。

### 7. 认证授权

内置 OAuth2 认证服务，支持密码模式和刷新令牌模式。

### 8. 数据库访问

使用 GORM ORM 框架，支持 PostgreSQL 数据库，提供连接池管理和迁移工具。

### 9. 缓存系统

集成 Redis 缓存，提供缓存穿透、击穿、雪崩防护机制。

## 十六、架构设计

### 1. 分层架构

项目采用清晰的分层架构设计：

- **API层**：处理HTTP请求和响应
- **Service层**：实现业务逻辑
- **Repository层**：处理数据访问
- **Model层**：定义数据模型

### 2. 微服务拆分

- **Gateway服务**：API网关，负责请求路由、熔断和限流
- **Auth服务**：认证授权服务，处理用户认证和令牌管理
- **User服务**：用户管理服务，处理用户相关业务逻辑

### 3. 共享组件

- **Config**：统一配置管理
- **DB**：数据库访问封装
- **Discovery**：服务发现封装
- **Logger**：日志系统封装
- **Middleware**：通用中间件

## 十七、部署指南

使用 Docker Compose 快速部署整套系统：

```bash
make run
```

或者手动执行：

```bash
cd deploy/docker
docker-compose up -d
```

## 十八、贡献指南

欢迎提交 Issue 和 Pull Request 来改进项目。
