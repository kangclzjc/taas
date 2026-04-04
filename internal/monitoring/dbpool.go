package monitoring

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// DBPoolCollector exposes pgxpool connection-pool statistics to Prometheus.
type DBPoolCollector struct {
	pool            *pgxpool.Pool
	totalConns      *prometheus.Desc
	acquiredConns   *prometheus.Desc
	idleConns       *prometheus.Desc
	maxConns        *prometheus.Desc
	acquireCount    *prometheus.Desc
	acquireDuration *prometheus.Desc
}

// NewDBPoolCollector creates a Prometheus collector that reads pool stats from
// the supplied pgxpool.Pool. All metric names are prefixed with namespace.
func NewDBPoolCollector(pool *pgxpool.Pool, namespace string) *DBPoolCollector {
	return &DBPoolCollector{
		pool:            pool,
		totalConns:      prometheus.NewDesc(namespace+"_db_pool_total_conns", "Total connections", nil, nil),
		acquiredConns:   prometheus.NewDesc(namespace+"_db_pool_acquired_conns", "Acquired connections", nil, nil),
		idleConns:       prometheus.NewDesc(namespace+"_db_pool_idle_conns", "Idle connections", nil, nil),
		maxConns:        prometheus.NewDesc(namespace+"_db_pool_max_conns", "Max connections", nil, nil),
		acquireCount:    prometheus.NewDesc(namespace+"_db_pool_acquire_total", "Total acquire calls", nil, nil),
		acquireDuration: prometheus.NewDesc(namespace+"_db_pool_acquire_duration_ns", "Total acquire wait time in ns", nil, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *DBPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalConns
	ch <- c.acquiredConns
	ch <- c.idleConns
	ch <- c.maxConns
	ch <- c.acquireCount
	ch <- c.acquireDuration
}

// Collect implements prometheus.Collector.
func (c *DBPoolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(stat.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.acquiredConns, prometheus.GaugeValue, float64(stat.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(stat.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(stat.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.acquireCount, prometheus.CounterValue, float64(stat.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireDuration, prometheus.CounterValue, float64(stat.AcquireDuration()))
}
