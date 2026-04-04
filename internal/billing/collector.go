package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Collector subscribes to NATS usage events and aggregates them into PostgreSQL.
type Collector struct {
	db     *pgxpool.Pool
	js     nats.JetStreamContext
	logger *zap.Logger
}

func NewCollector(db *pgxpool.Pool, js nats.JetStreamContext, logger *zap.Logger) *Collector {
	return &Collector{db: db, js: js, logger: logger}
}

// Start begins consuming usage events from NATS JetStream.
func (c *Collector) Start(ctx context.Context) error {
	// Ensure stream exists
	_, err := c.js.AddStream(&nats.StreamConfig{
		Name:     streamTaasEvents,
		Subjects: []string{"usage.>"},
		MaxAge:   7 * 24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("creating stream: %w", err)
	}

	sub, err := c.js.Subscribe(subjectUsageRecorded, func(msg *nats.Msg) {
		var event UsageEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			c.logger.Error("unmarshalling usage event", zap.Error(err))
			msg.Nak() //nolint:errcheck
			return
		}

		if err := c.record(ctx, &event); err != nil {
			c.logger.Error("recording usage", zap.Error(err), zap.String("request_id", event.RequestID))
			msg.Nak() //nolint:errcheck
			return
		}

		msg.Ack() //nolint:errcheck
	}, nats.Durable("usage-collector"), nats.ManualAck(), nats.AckWait(30*time.Second))

	if err != nil {
		return fmt.Errorf("subscribing: %w", err)
	}

	<-ctx.Done()
	return sub.Unsubscribe()
}

// record inserts a usage event into the database.
func (c *Collector) record(ctx context.Context, event *UsageEvent) error {
	_, err := c.db.Exec(ctx,
		`INSERT INTO usage_records (request_id, token_id, user_id, org_id, model_id, deployment_id,
		 prompt_tokens, completion_tokens, total_tokens, latency_ms, ttft_ms, status, error_code,
		 cost_usd, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT (request_id) DO NOTHING`,
		event.RequestID, event.TokenID, event.UserID, event.OrgID, event.ModelID, event.DeploymentID,
		event.PromptTokens, event.CompletionTokens, event.TotalTokens, event.LatencyMs, event.TTFTMs,
		event.Status, event.ErrorCode, event.CostUSD, event.Timestamp,
	)
	return err
}
