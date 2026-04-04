package monitoring

import (
	"testing"
)

func TestNewMetrics(t *testing.T) {
	// Use a unique namespace per test to avoid "duplicate metrics collector registration"
	// from prometheus global default registry when running multiple test runs.
	m := NewMetrics("test_metrics")

	if m.HTTPRequestsTotal == nil {
		t.Error("HTTPRequestsTotal is nil")
	}
	if m.HTTPRequestDuration == nil {
		t.Error("HTTPRequestDuration is nil")
	}
	if m.InferenceRequestsTotal == nil {
		t.Error("InferenceRequestsTotal is nil")
	}
	if m.InferenceLatency == nil {
		t.Error("InferenceLatency is nil")
	}
	if m.InferenceTTFT == nil {
		t.Error("InferenceTTFT is nil")
	}
	if m.InferenceTokensTotal == nil {
		t.Error("InferenceTokensTotal is nil")
	}
	if m.InferenceActiveRequests == nil {
		t.Error("InferenceActiveRequests is nil")
	}
	if m.TokenValidationsTotal == nil {
		t.Error("TokenValidationsTotal is nil")
	}
	if m.RateLimitHitsTotal == nil {
		t.Error("RateLimitHitsTotal is nil")
	}
	if m.ActiveDeployments == nil {
		t.Error("ActiveDeployments is nil")
	}
	if m.CostPerOrgUSD == nil {
		t.Error("CostPerOrgUSD is nil")
	}
}
