package token

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/litellm"
)

// LiteLLMService wraps the base token Service and syncs operations with LiteLLM Proxy.
//
// When a TaaS token is created, a corresponding LiteLLM virtual key is generated.
// When a TaaS token is revoked, the LiteLLM virtual key is deleted.
// When a TaaS token is rotated, the old LiteLLM key is deleted and a new one is created.
//
// This replaces the need for TaaS to handle API key auth and rate limiting itself —
// LiteLLM Proxy takes over those responsibilities with its virtual key system.
type LiteLLMService struct {
	*Service
	litellm *litellm.AdminClient
	logger  *zap.Logger
}

// NewLiteLLMService creates a token service that syncs with LiteLLM Proxy.
// If litellmClient is nil, falls back to base Service behavior (no LiteLLM sync).
func NewLiteLLMService(base *Service, litellmClient *litellm.AdminClient, logger *zap.Logger) *LiteLLMService {
	return &LiteLLMService{
		Service: base,
		litellm: litellmClient,
		logger:  logger,
	}
}

// Create generates a TaaS token and syncs it as a LiteLLM virtual key.
// The returned RawToken is the LiteLLM key (sk-...) if LiteLLM is enabled,
// or the TaaS token (taas_...) if running without LiteLLM.
func (s *LiteLLMService) Create(ctx context.Context, userID, orgID uuid.UUID, req CreateRequest) (*CreateResult, error) {
	// First create in TaaS DB
	result, err := s.Service.Create(ctx, userID, orgID, req)
	if err != nil {
		return nil, err
	}

	// If LiteLLM is not configured, return TaaS-native token
	if s.litellm == nil {
		return result, nil
	}

	// Sync to LiteLLM: create a virtual key with matching config
	var rpm, tpm *int
	if req.RateLimitRPM > 0 {
		rpm = &req.RateLimitRPM
	}
	if req.RateLimitTPM > 0 {
		tpm = &req.RateLimitTPM
	}
	var maxBudget *float64
	if req.BudgetLimitUSD > 0 {
		maxBudget = &req.BudgetLimitUSD
	}

	litellmReq := litellm.GenerateKeyRequest{
		KeyAlias:  result.Token.ID.String(), // Use TaaS token ID as alias for correlation
		UserID:    userID.String(),
		TeamID:    orgID.String(),
		Models:    req.AllowedModels,
		MaxBudget: maxBudget,
		RPM:       rpm,
		TPM:       tpm,
		Metadata: map[string]string{
			"taas_token_id": result.Token.ID.String(),
			"sla_tier":      result.Token.SLATier,
			"token_name":    req.Name,
		},
	}

	litellmResp, err := s.litellm.GenerateKey(ctx, litellmReq)
	if err != nil {
		// LiteLLM sync failed — log but don't fail the TaaS token creation.
		// The token exists in TaaS DB and can be synced later.
		s.logger.Error("failed to sync token to LiteLLM, token created in TaaS only",
			zap.Error(err),
			zap.String("token_id", result.Token.ID.String()),
		)
		return result, nil
	}

	// Store LiteLLM key reference in TaaS token metadata
	if err := s.Service.repo.SetLiteLLMKeyToken(ctx, result.Token.ID, litellmResp.Token); err != nil {
		s.logger.Error("failed to save LiteLLM key reference",
			zap.Error(err),
			zap.String("token_id", result.Token.ID.String()),
			zap.String("litellm_token", litellmResp.Token),
		)
	}

	// Return the LiteLLM key as the user-facing API key
	result.RawToken = litellmResp.Key
	s.logger.Info("token synced to LiteLLM",
		zap.String("token_id", result.Token.ID.String()),
		zap.String("litellm_token", litellmResp.Token),
	)

	return result, nil
}

// Revoke deactivates a TaaS token and deletes the corresponding LiteLLM virtual key.
func (s *LiteLLMService) Revoke(ctx context.Context, tokenID, orgID uuid.UUID) error {
	// Get LiteLLM key reference before revoking
	var litellmKeyToken string
	if s.litellm != nil {
		t, err := s.Service.repo.GetByID(ctx, tokenID)
		if err == nil && t != nil {
			litellmKeyToken = t.LiteLLMKeyToken
		}
	}

	// Revoke in TaaS
	if err := s.Service.Revoke(ctx, tokenID, orgID); err != nil {
		return err
	}

	// Delete from LiteLLM
	if s.litellm != nil && litellmKeyToken != "" {
		if err := s.litellm.DeleteKey(ctx, litellmKeyToken); err != nil {
			s.logger.Error("failed to delete LiteLLM virtual key",
				zap.Error(err),
				zap.String("token_id", tokenID.String()),
				zap.String("litellm_token", litellmKeyToken),
			)
			// Don't fail — TaaS token is already revoked
		}
	}

	return nil
}

// Rotate generates a new API key for an existing token, syncing with LiteLLM.
// This deletes the old LiteLLM key and creates a new one.
func (s *LiteLLMService) Rotate(ctx context.Context, tokenID, orgID uuid.UUID) (string, error) {
	if s.litellm == nil {
		return s.Service.Rotate(ctx, tokenID, orgID)
	}

	// Get existing token info
	t, err := s.Service.repo.GetByID(ctx, tokenID)
	if err != nil || t == nil || t.OrgID != orgID {
		return "", fmt.Errorf("token not found")
	}
	if !t.IsActive {
		return "", fmt.Errorf("cannot rotate a revoked token")
	}

	// Delete old LiteLLM key
	if t.LiteLLMKeyToken != "" {
		if err := s.litellm.DeleteKey(ctx, t.LiteLLMKeyToken); err != nil {
			s.logger.Warn("failed to delete old LiteLLM key during rotation", zap.Error(err))
		}
	}

	// Rotate in TaaS (new hash/prefix)
	newTaasRaw, err := s.Service.Rotate(ctx, tokenID, orgID)
	if err != nil {
		return "", err
	}

	// Create new LiteLLM key
	litellmReq := litellm.GenerateKeyRequest{
		KeyAlias:  t.ID.String(),
		UserID:    t.UserID.String(),
		TeamID:    t.OrgID.String(),
		Models:    t.AllowedModels,
		Metadata: map[string]string{
			"taas_token_id": t.ID.String(),
			"sla_tier":      t.SLATier,
			"token_name":    t.Name,
		},
	}

	var rpm, tpm *int
	if t.RateLimitRPM > 0 {
		rpm = &t.RateLimitRPM
		litellmReq.RPM = rpm
	}
	if t.RateLimitTPM > 0 {
		tpm = &t.RateLimitTPM
		litellmReq.TPM = tpm
	}
	if t.BudgetLimitUSD > 0 {
		litellmReq.MaxBudget = &t.BudgetLimitUSD
	}

	litellmResp, err := s.litellm.GenerateKey(ctx, litellmReq)
	if err != nil {
		s.logger.Error("failed to create new LiteLLM key during rotation, returning TaaS key",
			zap.Error(err),
			zap.String("token_id", tokenID.String()),
		)
		return newTaasRaw, nil
	}

	// Update LiteLLM key reference
	if err := s.Service.repo.SetLiteLLMKeyToken(ctx, tokenID, litellmResp.Token); err != nil {
		s.logger.Error("failed to save new LiteLLM key reference", zap.Error(err))
	}

	return litellmResp.Key, nil
}
