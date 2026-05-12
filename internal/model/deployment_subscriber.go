package model

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/natsutil"
)

// StartDeploymentStatusConsumer subscribes to JetStream deployment.status.updated and
// reconciles the deployments table (operator / mock operator publishes here).
func StartDeploymentStatusConsumer(
	ctx context.Context,
	js nats.JetStreamContext,
	svc *Service,
	logger *zap.Logger,
	onRunning func(context.Context, uuid.UUID, string) error,
) (*nats.Subscription, error) {
	if err := natsutil.EnsureTAASEventsStream(js); err != nil {
		return nil, err
	}

	sub, err := natsutil.SubscribePushDurable(js, "deployment.status.updated", func(msg *nats.Msg) {
		var payload struct {
			DeploymentID string `json:"deployment_id"`
			Status       string `json:"status"`
			EndpointURL  string `json:"endpoint_url"`
			ErrorMessage string `json:"error_message"`
			Replicas     int    `json:"replicas"`
		}
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			logger.Warn("deployment status: bad json", zap.Error(err))
			_ = msg.Nak()
			return
		}
		did, err := uuid.Parse(payload.DeploymentID)
		if err != nil {
			logger.Warn("deployment status: bad deployment_id", zap.String("id", payload.DeploymentID))
			_ = msg.Term()
			return
		}

		dbCtx := context.Background()
		switch payload.Status {
		case "running":
			rep := payload.Replicas
			if rep < 1 {
				rep = 1
			}
			if err := svc.CompleteSimulatedDeployment(dbCtx, did, rep, payload.EndpointURL); err != nil {
				logger.Error("deployment status: mark running", zap.Error(err), zap.String("deployment_id", did.String()))
				_ = msg.Nak()
				return
			}
			if onRunning != nil && payload.EndpointURL != "" {
				if err := onRunning(dbCtx, did, payload.EndpointURL); err != nil {
					logger.Warn("deployment status: running hook failed",
						zap.Error(err),
						zap.String("deployment_id", did.String()),
					)
				}
			}
			logger.Info("deployment status: running", zap.String("deployment_id", did.String()), zap.String("endpoint", payload.EndpointURL))
			_ = msg.Ack()
		case "failed":
			msgErr := payload.ErrorMessage
			if msgErr == "" {
				msgErr = "deployment failed"
			}
			if err := svc.MarkDeploymentFailed(dbCtx, did, msgErr); err != nil {
				logger.Error("deployment status: mark failed", zap.Error(err), zap.String("deployment_id", did.String()))
				_ = msg.Nak()
				return
			}
			logger.Info("deployment status: failed", zap.String("deployment_id", did.String()))
			_ = msg.Ack()
		default:
			logger.Warn("deployment status: unknown status", zap.String("status", payload.Status))
			_ = msg.Ack()
		}
	}, natsutil.StreamTAASEvents, "taas-gateway-deploy-status", nats.ManualAck())
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		if uerr := sub.Unsubscribe(); uerr != nil {
			logger.Warn("deployment status consumer unsubscribe", zap.Error(uerr))
		}
	}()

	return sub, nil
}
