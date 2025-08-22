# easyms (Go-kit + Consul + OAuth2 + Circuit Breaker)

## 项目背景

> **像拼乐高一样搭建微服务**

本项目灵感来源于乐高积木的模块化思想，旨在通过**Go-kit**、**Consul**、**OAuth2** 和 **Circuit Breaker** 这些“技术积木”，帮助开发者快速构建安全可靠的微服务架构。就像拼乐高时每个小颗粒都有特定功能，每个组件都在系统中扮演着关键角色。

---
以下是当前相关的博文，结合博文，可快速了解本系统的核心功能和实现原理, 后续会持续更新，欢迎大家扫码关注“代码扳手”公众号，获取更多技术交流信息。

-[从混乱到秩序：基于 Go kit 重塑 easyms 微服务架构的实战指南](https://mp.weixin.qq.com/s/C1vnLaCn_sfQntmxMzXlIg)

![wx.jpg](wx.jpg)

## 核心功能

### 服务组件

- **Consul** 🧱
  - 服务注册与发现
  - 配置中心（KV）

- **Auth Service** 🔐
  - OAuth2
  - Postgres 存储 client 和 token
  - JWT RS256 签名认证

- **User Service** 👤
  - 简单用户服务，返回 profile 数据
  - 支持健康检查

- **API Gateway** 🌉
  - 服务发现 + 负载均衡 (RoundRobin + Retry)
  - 熔断保护 (mercari/go-circuitbreaker)
  - Prometheus metrics 暴露 `/metrics`
  - 请求认证拦截

## 技术亮点

### 微服务架构

easy```text
// 示例：服务注册代码
// 实际代码应导入 "github.com/hashicorp/consul/api"
// 并通过 consul.NewClient 创建客户端
package main

import (
	"github.com/hashicorp/consul/api"
)

func registerService(client *api.Client, serviceID string) error {
    registration := &api.AgentServiceRegistration{
        ID:   serviceID,
        Name: "auth-svc",
        Port: 8080,
        Check: &api.AgentServiceCheck{
            HTTP:     "http://localhost:8080/health",
            Interval: "10s",
        },
    }
    return client.Agent().ServiceRegister(registration)
}
```

### 认证授权

使用 OAuth2 授权服务，支持 client_credentials 模式，提供 JWT Token 颁发和 JWK 公钥暴露功能。

### 熔断保护

通过 [mercari/go-circuitbreaker](https://github.com/mercari/go-circuitbreaker) 实现服务调用熔断机制，防止雪崩效应，确保系统稳定性。

## 开发体验

### 快速开始

#### 启动服务

```bash
# 构建并启动所有服务
docker compose up --build
```

首次启动时 `auth-svc` 会自动在 Postgres 中建表，并插入一个 demo client：

- client_id: `demo-client`
- client_secret: `demo-secret`
- redirect_uri: `http://localhost:10001/oauth/callback`
- scope: `user.read`

#### 获取 Access Token

使用客户端凭证模式：

```bash
curl -X POST "http://localhost:8080/oauth/token"   -u "demo-client:demo-secret"   -d "grant_type=client_credentials&scope=user.read"
```

返回示例：

```json
{
  "access_token": "eyJhbGciOi...",
  "expires_in": 3600,
  "scope": "user.read",
  "token_type": "bearer"
}
```

#### 调用用户服务 API (经由网关)

```bash
curl -H "Authorization: Bearer <ACCESS_TOKEN>"   http://localhost:10002/api/profile
```

返回：

```json
{"id":"u1","name":"Alice"}
```

#### 查看熔断指标

访问 Prometheus metrics：

```
http://localhost:8082/metrics
```

可以看到熔断状态：

```
circuit_breaker_state{instance="user-svc:8081"} 0
```

其中状态含义：  
- `0 = Closed` (正常)  
- `1 = Half-Open` (探测)  
- `2 = Open` (熔断中)  

#### 健康检查

- Auth Service: [http://localhost:10001/health](http://localhost:10001/health)  
- User Service: [http://localhost:10002/health](http://localhost:10002/health)  
- API Gateway: [http://localhost:10000/health](http://localhost:10000/health)  

## 避坑指南 ⚠️

### 服务发现配置

确保 Consul 配置中的 `key_path` 字段格式为 `easyms/<service_name>/<env>`，避免服务注册失败。

### 熔断器配置

合理设置熔断器阈值和超时时间，避免过于敏感导致误熔断。

### 环境变量配置

开发环境和生产环境的配置要区分，确保敏感信息不会泄露。

## 延伸玩法 🚀

- 添加新的 OAuth2 授权模式，如授权码模式
- 实现更复杂的负载均衡策略
- 集成日志收集系统（如 ELK）
- 实现分布式追踪（如 Jaeger）

## 互动设计 💬

- 你有尝试过使用其他熔断库吗？体验如何？
- 如果要增加服务间通信的加密，你会怎么做？

## 项目地址

GitHub: [https://github.com/louis-xie-programmer/easyms.golang](https://github.com/louis-xie-programmer/easyms.golang)
Gitee: [https://gitee.com/louis_xie/easyms.golang](https://gitee.com/louis_xie/easyms.golang)

## License

MIT

### OAuth2 (JWT RS256) Quickstart
See `cmd/auth-svc` for the new auth service. Use demo client `demo-client/demo-secret` and user `alice/alicepwd`.