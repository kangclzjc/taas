package token

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TokenService is the interface for token business logic.
// Both *Service (base) and *LiteLLMService (with virtual key sync) implement this.
type TokenService interface {
	Create(ctx context.Context, userID, orgID uuid.UUID, req CreateRequest) (*CreateResult, error)
	List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*Token, error)
	Revoke(ctx context.Context, tokenID, orgID uuid.UUID) error
	Rotate(ctx context.Context, tokenID, orgID uuid.UUID) (string, error)
}

// NewLiteLLMHandler creates a Handler backed by LiteLLMService.
// The returned Handler uses LiteLLMService's overridden Create/Revoke/Rotate methods
// which sync with LiteLLM Proxy virtual keys.
func NewLiteLLMHandler(svc *LiteLLMService, logger *zap.Logger) *Handler {
	return &Handler{
		svc:    svc,
		logger: logger,
	}
}

// Ensure LiteLLMService implements TokenService at compile time.
var _ TokenService = (*LiteLLMService)(nil)
var _ TokenService = (*Service)(nil)
