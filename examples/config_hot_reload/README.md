# 配置热更新示例

这个示例展示了如何在你的服务中使用配置热更新功能。

## 功能说明

1. 服务启动时会从Consul加载配置
2. 如果启用了配置热更新(`reload_on_changes: true`)，服务会监听配置变更
3. 当Consul中的配置发生变化时，服务会自动重新加载配置

## 运行示例

1. 确保Consul服务正在运行
2. 在Consul中添加配置键值，例如:
   - Key: `easyms/dev/example-svc.yaml`
   - Value: 
     ```yaml
     server:
       host: "localhost"
       port: 8080
     log:
       log_level: "info"
       log_type: "console"
     ```
3. 运行示例:
   ```
   go run main.go
   ```
4. 修改Consul中的配置，观察控制台输出

## 配置版本管理

你可以通过HTTP API管理配置版本:

1. 保存当前配置为新版本:
   ```bash
   curl -X POST http://localhost:PORT/config/version \
     -H "Content-Type: application/json" \
     -d '{"description":"版本描述"}'
   ```

2. 查看配置版本列表:
   ```bash
   curl http://localhost:PORT/config/versions
   ```

3. 回滚到指定版本:
   ```bash
   curl -X POST http://localhost:PORT/config/rollback/VERSION_ID
   ```

## 注意事项

1. 确保Consul服务地址配置正确
2. 确保服务有权限读取和写入Consul中的相应键
3. 配置热更新不会影响正在处理的请求，新配置仅对新请求生效