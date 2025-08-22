package registry

import (
    "fmt"
    "net"
    consul "github.com/hashicorp/consul/api"
)

// HostPort 解析 host:port
func HostPort(addr string) (string,int) {
    h, p, _ := net.SplitHostPort(addr)
    var port int
    fmt.Sscanf(p, "%d", &port)
    if h=="" { h = "127.0.0.1" }
    return h, port
}

// Register 将服务注册到 Consul
func Register(client *consul.Client, name, host string, port int, tags []string) error {
    check := &consul.AgentServiceCheck{HTTP: fmt.Sprintf("http://%s:%d/health", host, port), Interval: "5s", Timeout: "2s"}
    svc := &consul.AgentServiceRegistration{Name: name, Address: host, Port: port, Tags: tags, Check: check}
    return client.Agent().ServiceRegister(svc)
}
