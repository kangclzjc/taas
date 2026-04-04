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

// CostCalculator computes inference cost given token counts and model pricing.
type CostCalculator struct {
	// pricePerMTokens maps model_id → (prompt_price, completion_price) per million tokens
	prices map[string][2]float64
	// defaultPrice used when model not in map
	defaultPrice [2]float64
}

func NewCostCalculator() *CostCalculator {
	return &CostCalculator{
		prices: map[string][2]float64{
			// Example pricing (USD per million tokens)
			"llama-3-8b":   {0.10, 0.20},
			"llama-3-70b":  {0.50, 1.00},
			"mistral-7b":   {0.10, 0.20},
			"mixtral-8x7b": {0.40, 0.80},
		},
		defaultPrice: [2]float64{0.50, 1.00},
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
