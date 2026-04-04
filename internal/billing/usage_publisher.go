package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	subjectUsageRecorded = "usage.recorded"
	streamTaasEvents     = "TAAS_EVENTS"
)

// UsageEvent is published to NATS after each inference request.
type UsageEvent struct {
	RequestID        string    `json:"request_id"`
	TokenID          string    `json:"token_id"`
	UserID           string    `json:"user_id"`
	OrgID            string    `json:"org_id"`
	ModelID          string    `json:"model_id"`
	DeploymentID     string    `json:"deployment_id"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	LatencyMs        int       `json:"latency_ms"`
	TTFTMs           int       `json:"ttft_ms"`
	Status           string    `json:"status"` // success|error|throttled
	ErrorCode        string    `json:"error_code,omitempty"`
	CostUSD          float64   `json:"cost_usd"`
	Timestamp        time.Time `json:"timestamp"`
}

// Publisher publishes usage events to NATS JetStream.
type Publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

// Publish sends a usage event. Non-blocking — uses JetStream for durability.
func (p *Publisher) Publish(ctx context.Context, event UsageEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal usage event: %w", err)
	}

	_, err = p.js.Publish(subjectUsageRecorded, data,
		nats.Context(ctx),
		nats.MsgId(event.RequestID), // deduplication
	)
	return err
}

// PricingConfig holds model pricing configuration for the CostCalculator.
type PricingConfig struct {
	Prices       map[string][2]float64
	DefaultPrice [2]float64
}

// CostCalculator computes inference cost given token counts and model pricing.
type CostCalculator struct {
	// pricePerMTokens maps model_id → (prompt_price, completion_price) per million tokens
	prices map[string][2]float64
	// defaultPrice used when model not in map
	defaultPrice [2]float64
}

func NewCostCalculator(cfg PricingConfig) *CostCalculator {
	prices := cfg.Prices
	if prices == nil {
		prices = make(map[string][2]float64)
	}
	return &CostCalculator{
		prices:       prices,
		defaultPrice: cfg.DefaultPrice,
	}
}

// Calculate returns the cost in USD for given token counts and model.
func (c *CostCalculator) Calculate(modelID string, promptTokens, completionTokens int) float64 {
	price, ok := c.prices[modelID]
	if !ok {
		price = c.defaultPrice
	}
	promptCost := float64(promptTokens) / 1_000_000 * price[0]
	completionCost := float64(completionTokens) / 1_000_000 * price[1]
	return promptCost + completionCost
}
