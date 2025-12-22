package db

import (
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

	operation := db.Statement.SQL.String()
	if len(operation) > 50 { // 简化操作名
		operation = operation[:50]
	}

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
