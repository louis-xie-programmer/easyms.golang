// Package logger 提供了统一的日志框架。
package logger

import (
	"bytes"
	"easyms/internal/shared/models"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

// LokiLogger 是 BackendLogger 接口的一个实现，它将日志发送到 Grafana Loki。
type LokiLogger struct {
	url      string       // Loki 的 HTTP 推送端点 (例如 "http://loki:3100/loki/api/v1/push")
	service  string       // 服务名称，将作为 Loki 的一个标签
	username string       // Loki 认证的用户名 (如果需要)
	password string       // Loki 认证的密码 (如果需要)
	client   *http.Client // 用于发送日志的 HTTP 客户端
}

// NewLokiLogger 创建一个新的 LokiLogger 实例。
func NewLokiLogger(service string, cfg models.LokiConfig) BackendLogger {
	return &LokiLogger{
		url:      cfg.URL,
		service:  service,
		username: cfg.Username,
		password: cfg.Password,
		client: &http.Client{
			Timeout: 5 * time.Second, // 设置 HTTP 请求超时时间
		},
	}
}

// Log 实现了 BackendLogger 接口的 Log 方法。
// 它将一批日志条目构建成 Loki 的数据格式，并通过 HTTP 发送。
func (l *LokiLogger) Log(logs []*LogEntry) error {
	// 将日志条目按标签分组，构建成 Loki 的 "streams" 格式
	streams := l.buildStreams(logs)
	if len(streams) == 0 {
		return nil
	}

	// 构建 Loki API 要求的最外层 JSON 结构
	reqBody := struct {
		Streams []interface{} `json:"streams"`
	}{
		Streams: streams,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化 Loki 请求体失败: %w", err)
	}

	// 使用带重试的逻辑发送日志
	return l.sendWithRetry(jsonBody)
}

// buildStreams 将一批 LogEntry 转换为 Loki 的 "streams" 格式。
// Loki 的数据模型由 "streams" 组成，每个 stream 是一组具有相同标签 (labels) 的日志。
// 在这里，我们使用 "app" (服务名) 和 "module" (模块名) 作为标签。
func (l *LokiLogger) buildStreams(logs []*LogEntry) []interface{} {
	// 使用 map 将日志按模块名进行分组
	streamsMap := make(map[string][][]string)
	for _, entry := range logs {
		// 将整个 LogEntry 序列化为 JSON 字符串，作为 Loki 日志行的内容
		logLine, err := json.Marshal(entry)
		if err != nil {
			fallbackLogger.Error().Err(err).Msg("为 Loki 序列化日志条目失败")
			continue
		}
		// Loki 的每条日志是一个 [时间戳, 日志内容] 的元组
		val := []string{strconv.FormatInt(entry.Timestamp.UnixNano(), 10), string(logLine)}
		streamsMap[entry.Module] = append(streamsMap[entry.Module], val)
	}

	// 将分组后的 map 转换为 Loki API 需要的切片格式
	var streams []interface{}
	for module, values := range streamsMap {
		streams = append(streams, struct {
			Stream map[string]string `json:"stream"` // 标签集
			Values [][]string        `json:"values"` // 日志条目列表
		}{
			Stream: map[string]string{
				"app":    l.service,
				"module": module,
			},
			Values: values,
		})
	}
	return streams
}

// sendWithRetry 使用指数退避策略来发送日志，以增加发送的成功率。
func (l *LokiLogger) sendWithRetry(body []byte) error {
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest("POST", l.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("创建 Loki 请求失败: %w", err)
		}
		// 如果配置了用户名和密码，则设置 Basic Auth
		if l.username != "" || l.password != "" {
			req.SetBasicAuth(l.username, l.password)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := l.client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			// HTTP 状态码 2xx 表示成功
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil // 发送成功
			}
			lastErr = errors.New("非预期的 HTTP 状态码: " + resp.Status)
		} else {
			lastErr = err
		}

		// 如果还未达到最大重试次数，则进行指数退避等待
		if i < maxRetries-1 {
			// 等待时间：1s, 2s, 4s, ...
			time.Sleep(time.Duration(math.Pow(2, float64(i))) * time.Second)
		}
	}

	// 在多次重试后仍然失败，使用备用 logger 记录最终错误
	fallbackLogger.Error().Err(lastErr).Msgf("在 %d 次重试后，发送日志到 Loki 失败", maxRetries)
	// 不向上层返回错误，因为错误已由备用 logger 处理，避免阻塞上层日志框架
	return nil
}
