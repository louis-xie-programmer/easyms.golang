package db

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gorm.io/gorm"
)

var (
	dbRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "db_requests_total",
			Help: "Total number of database requests.",
		},
		[]string{"db_type", "operation", "table", "status"},
	)

	dbRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_request_duration_seconds",
			Help:    "Duration of database requests.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2.5, 5},
		},
		[]string{"db_type", "operation", "table"},
	)
)

// MetricsPlugin 是一个 GORM 插件，用于收集数据库操作的 Prometheus 指标。
type MetricsPlugin struct {
	DBType string
}

func (p *MetricsPlugin) Name() string {
	return "prometheusMetrics"
}

func (p *MetricsPlugin) Initialize(db *gorm.DB) error {
	// 注册回调：在操作开始前记录时间
	db.Callback().Create().Before("gorm:create").Register("metrics:before_create", p.before)
	db.Callback().Query().Before("gorm:query").Register("metrics:before_query", p.before)
	db.Callback().Update().Before("gorm:update").Register("metrics:before_update", p.before)
	db.Callback().Delete().Before("gorm:delete").Register("metrics:before_delete", p.before)
	db.Callback().Row().Before("gorm:row").Register("metrics:before_row", p.before)
	db.Callback().Raw().Before("gorm:raw").Register("metrics:before_raw", p.before)

	// 注册回调：在操作结束后计算耗时和状态
	db.Callback().Create().After("gorm:create").Register("metrics:after_create", p.after)
	db.Callback().Query().After("gorm:query").Register("metrics:after_query", p.after)
	db.Callback().Update().After("gorm:update").Register("metrics:after_update", p.after)
	db.Callback().Delete().After("gorm:delete").Register("metrics:after_delete", p.after)
	db.Callback().Row().After("gorm:row").Register("metrics:after_row", p.after)
	db.Callback().Raw().After("gorm:raw").Register("metrics:after_raw", p.after)

	return nil
}

const startTimeKey = "metrics:start_time"

func (p *MetricsPlugin) before(db *gorm.DB) {
	db.Set(startTimeKey, time.Now())
}

func (p *MetricsPlugin) after(db *gorm.DB) {
	startTime, ok := db.Get(startTimeKey)
	if !ok {
		return
	}
	st, ok := startTime.(time.Time)
	if !ok {
		return
	}

	// 优化 Operation Label：使用 "操作类型_表名" 或 SQL 哈希
	operation := getOperationLabel(db)

	table := db.Statement.Table
	if table == "" {
		table = "unknown"
	}

	status := "success"
	if db.Error != nil && db.Error != gorm.ErrRecordNotFound {
		status = "error"
	}

	duration := time.Since(st).Seconds()

	// 记录耗时
	dbRequestDuration.WithLabelValues(p.DBType, operation, table).Observe(duration)
	// 记录总数和状态
	dbRequestsTotal.WithLabelValues(p.DBType, operation, table, status).Inc()
}

// getOperationLabel 生成低基数的 Operation Label
func getOperationLabel(db *gorm.DB) string {
	// 1. 优先尝试使用 SQL 模板哈希（如果能获取到 SQL 模板）
	// GORM 的 Statement.SQL.String() 通常是带参数占位符的 SQL，但也可能包含具体值
	// 为了安全起见，我们主要依赖操作类型 + 表名

	// 获取 SQL 语句的前几个单词作为操作类型
	sql := strings.TrimSpace(strings.ToUpper(db.Statement.SQL.String()))
	parts := strings.Fields(sql)
	if len(parts) > 0 {
		op := parts[0]
		// 常见 SQL 动词白名单
		switch op {
		case "SELECT", "INSERT", "UPDATE", "DELETE", "BEGIN", "COMMIT", "ROLLBACK":
			// 如果有表名，组合成 SELECT_users 这种形式
			if db.Statement.Table != "" {
				return op + "_" + db.Statement.Table
			}
			return op
		}
	}

	// 如果无法识别，回退到使用 SQL 哈希（截取前8位）
	// 这种方式虽然基数可能稍高，但比原始 SQL 好得多
	hash := sha256.Sum256([]byte(sql))
	return "SQL_" + hex.EncodeToString(hash[:])[:8]
}

// 辅助函数：移除 SQL 中的多余空格和换行
var spaceRegexp = regexp.MustCompile(`\s+`)

func normalizeSQL(sql string) string {
	return strings.TrimSpace(spaceRegexp.ReplaceAllString(sql, " "))
}
