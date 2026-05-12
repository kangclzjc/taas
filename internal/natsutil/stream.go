package natsutil

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const StreamTAASEvents = "TAAS_EVENTS"

// EnsureTAASEventsStream creates or updates the JetStream stream used by TaaS
// (usage, model deploy orchestration, deployment status callbacks).
func EnsureTAASEventsStream(js nats.JetStreamContext) error {
	cfg := &nats.StreamConfig{
		Name:     StreamTAASEvents,
		Subjects: []string{"usage.>", "model.deploy.>", "deployment.>"},
		MaxAge:   7 * 24 * time.Hour,
	}
	if _, err := js.StreamInfo(StreamTAASEvents); err != nil {
		if _, addErr := js.AddStream(cfg); addErr != nil {
			return fmt.Errorf("add jetstream stream %s: %w", StreamTAASEvents, addErr)
		}
		return nil
	}
	if _, err := js.UpdateStream(cfg); err != nil {
		return fmt.Errorf("update jetstream stream %s: %w", StreamTAASEvents, err)
	}
	return nil
}
