// loki.go 实现Loki日志后端

package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/louis-xie-programmer/easyms/pkg/config"
	"net/http"
	"strconv"
)

// LokiLogger 实现Loki日志后端
type LokiLogger struct {
	url      string
	service  string
	username string
	password string
}

// Log 将日志条目发送到Loki日志系统
// 参数:
//
//	logs: 要发送的日志条目切片，包含模块、级别、消息等信息
//
// 返回值:
//
//	error: 操作成功返回nil，失败返回具体错误
//
// 实现流程：
// 1. 构建Loki兼容的日志流结构
// 2. 转换日志条目为Loki所需的格式
// 3. 创建并发送包含认证信息的HTTP请求
// 4. 处理响应结果及可能的错误
func (l *LokiLogger) Log(logs []LogEntry) error {
	// 初始化Loki日志流结构
	stream := struct {
		Stream map[string]string `json:"stream"`
		Values [][]string        `json:"values"`
	}{}
	// 设置服务标识为日志标签
	stream.Stream = map[string]string{
		"app": l.service,
	}

	// 遍历日志条目，转换为Loki兼容格式
	for _, entry := range logs {
		// 构建日志条目基础字段
		datas, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		vals := []string{
			strconv.FormatInt(entry.Timestamp.UnixNano(), 10),
			fmt.Sprintf("%s", datas),
		}

		// 添加处理后的日志条目到流数据
		stream.Values = append(
			stream.Values,
			vals,
		)
	}

	// 构建请求体结构
	reqBody := struct {
		Streams []interface{} `json:"streams"`
	}{
		// 将日志流封装到请求体中
		Streams: []interface{}{stream},
	}

	// 序列化请求体为JSON格式
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	fmt.Printf("jsonBody: %s", jsonBody)

	// 创建HTTP POST请求
	req, err := http.NewRequest("POST", l.url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	// 添加基础认证信息
	req.SetBasicAuth(l.username, l.password)
	// 设置请求内容类型
	req.Header.Set("Content-Type", "application/json")

	// 创建HTTP客户端并发送请求
	client := &http.Client{}

	// 执行请求并获取响应
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	// 确保响应体正确关闭
	defer resp.Body.Close()

	// 验证响应状态码是否为预期的成功状态
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// NewLokiLogger 创建并返回一个新的LokiLogger实例
// 参数:
//
//	service: 服务名称，用于标识日志来源(app 服务名称)
//	cfg:     Loki配置对象，包含URL、用户名和密码
//
// 返回值:
//
//	*LokiLogger: 初始化后的LokiLogger指针
//
// 初始化结构体字段并配置HTTP客户端，设置5秒超时限制
func NewLokiLogger(service string, cfg config.AppConfig) *LokiLogger {
	return &LokiLogger{
		url:      cfg.Loki["Url"],
		service:  service,
		username: cfg.Loki["username"],
		password: cfg.Loki["password"],
	}
}
