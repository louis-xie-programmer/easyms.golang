// loki_logger.go 实现Loki日志后端
package logger

import (
	"bytes"
	"easyms/internal/shared/entities"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

// LokiLogger 实现Loki日志后端
type LokiLogger struct {
	url      string
	service  string
	username string
	password string
	client   *http.Client
}

// NewLokiLogger 创建新的Loki日志记录器
func NewLokiLogger(service string, cfg entities.LokiConfig) BackendLogger {
	return &LokiLogger{
		url:      cfg.URL,
		service:  service,
		username: cfg.Username,
		password: cfg.Password,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Log 实现 BackendLogger 接口
func (l *LokiLogger) Log(logs []*LogEntry) error {
	streams := l.buildStreams(logs)
	if len(streams) == 0 {
		return nil
	}

	reqBody := struct {
		Streams []interface{} `json:"streams"`
	}{
		Streams: streams,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal loki request body: %w", err)
	}

	return l.sendWithRetry(jsonBody)
}

// buildStreams 将日志条目按模块分组构建为Loki流
func (l *LokiLogger) buildStreams(logs []*LogEntry) []interface{} {
	// 按 module 分组
	streamsMap := make(map[string][][]string)
	for _, entry := range logs {
		logLine, err := json.Marshal(entry)
		if err != nil {
			fallbackLogger.Error().Err(err).Msg("Failed to marshal log entry for Loki")
			continue
		}
		val := []string{strconv.FormatInt(entry.Timestamp.UnixNano(), 10), string(logLine)}
		streamsMap[entry.Module] = append(streamsMap[entry.Module], val)
	}

	var streams []interface{}
	for module, values := range streamsMap {
		streams = append(streams, struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
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

// sendWithRetry 带重试逻辑的发送函数
func (l *LokiLogger) sendWithRetry(body []byte) error {
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest("POST", l.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create loki request: %w", err)
		}
		req.SetBasicAuth(l.username, l.password)
		req.Header.Set("Content-Type", "application/json")

		resp, err := l.client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil // Success
			}
			lastErr = errors.New("unexpected status code: " + resp.Status)
		} else {
			lastErr = err
		}

		// Exponential backoff
		if i < maxRetries-1 {
			time.Sleep(time.Duration(math.Pow(2, float64(i))) * time.Second)
		}
	}

	fallbackLogger.Error().Err(lastErr).Msgf("Failed to send logs to Loki after %d retries", maxRetries)
	// Do not return error to the caller, as it's already handled by fallback logger
	return nil
}
