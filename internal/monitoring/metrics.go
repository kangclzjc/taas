package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for TaaS services.
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// Inference metrics
	InferenceRequestsTotal   *prometheus.CounterVec
	InferenceLatency         *prometheus.HistogramVec
	InferenceTTFT            *prometheus.HistogramVec
	InferenceTokensTotal     *prometheus.CounterVec
	InferenceActiveRequests  *prometheus.GaugeVec

	// Token/quota metrics
	TokenValidationsTotal *prometheus.CounterVec
	RateLimitHitsTotal    *prometheus.CounterVec
	QuotaExceededTotal    *prometheus.CounterVec
	BudgetUsageRatio      *prometheus.GaugeVec

	// Deployment metrics
	ActiveDeployments  *prometheus.GaugeVec
	DeploymentReplicas *prometheus.GaugeVec

	// Cost metrics
	CostPerOrgUSD *prometheus.CounterVec
}

// NewMetrics registers and returns all TaaS Prometheus metrics.
func NewMetrics(namespace string) *Metrics {
	return &Metrics{
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"service", "method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request latency in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"service", "method", "path"},
		),
		InferenceRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "inference_requests_total",
				Help:      "Total inference requests",
			},
			[]string{"model_id", "org_id", "sla_tier", "status"},
		),
		InferenceLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "inference_latency_seconds",
				Help:      "End-to-end inference latency",
				Buckets:   []float64{.1, .25, .5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"model_id", "sla_tier"},
		),
		InferenceTTFT: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "inference_ttft_seconds",
				Help:      "Time to first token",
				Buckets:   []float64{.05, .1, .2, .5, 1, 2, 5},
			},
			[]string{"model_id", "sla_tier"},
		),
		InferenceTokensTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "inference_tokens_total",
				Help:      "Total tokens processed",
			},
			[]string{"model_id", "org_id", "token_type"}, // token_type: prompt|completion
		),
		InferenceActiveRequests: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "inference_active_requests",
				Help:      "Currently in-flight inference requests",
			},
			[]string{"deployment_id"},
		),
		TokenValidationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "token_validations_total",
				Help:      "API token validation attempts",
			},
			[]string{"result"}, // hit|miss|invalid|revoked|expired
		),
		RateLimitHitsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_hits_total",
				Help:      "Rate limit enforcement events",
			},
			[]string{"org_id", "limit_type"}, // rpm|tpm|concurrent
		),
		ActiveDeployments: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_deployments",
				Help:      "Number of active model deployments",
			},
			[]string{"org_id", "sla_tier"},
		),
		CostPerOrgUSD: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cost_usd_total",
				Help:      "Cumulative cost in USD",
			},
			[]string{"org_id", "model_id"},
		),
	}
}
