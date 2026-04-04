package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ModelShare represents a sharing grant for a model to another org.
type ModelShare struct {
	ID         uuid.UUID `json:"id"`
	ModelID    uuid.UUID `json:"model_id"`
	OwnerOrgID uuid.UUID `json:"owner_org_id"`
	TargetOrgID uuid.UUID `json:"target_org_id"`
	Permission string    `json:"permission"` // "read" or "deploy"
	CreatedAt  time.Time `json:"created_at"`
}

// SharingService manages model sharing between orgs.
type SharingService struct {
	db *pgxpool.Pool
}

func NewSharingService(db *pgxpool.Pool) *SharingService {
	return &SharingService{db: db}
}

// Grant creates a sharing grant for a model to a target org.
func (s *SharingService) Grant(ctx context.Context, modelID, ownerOrgID, targetOrgID uuid.UUID, permission string) error {
	if permission != "read" && permission != "deploy" {
		return fmt.Errorf("invalid permission: %s", permission)
	}

	share := &ModelShare{
		ID:          uuid.New(),
		ModelID:     modelID,
		OwnerOrgID:  ownerOrgID,
		TargetOrgID: targetOrgID,
		Permission:  permission,
		CreatedAt:   time.Now().UTC(),
	}

	_, err := s.db.Exec(ctx,
		`INSERT INTO model_shares (id, model_id, owner_org_id, target_org_id, permission, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (model_id, target_org_id) DO UPDATE SET permission = $5`,
		share.ID, share.ModelID, share.OwnerOrgID, share.TargetOrgID, share.Permission, share.CreatedAt,
	)
	return err
}

// Revoke removes a sharing grant.
func (s *SharingService) Revoke(ctx context.Context, modelID, targetOrgID uuid.UUID) error {
	ct, err := s.db.Exec(ctx,
		`DELETE FROM model_shares WHERE model_id = $1 AND target_org_id = $2`,
		modelID, targetOrgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("share not found")
	}
	return nil
}

// ListShares returns all orgs a model is shared with.
func (s *SharingService) ListShares(ctx context.Context, modelID uuid.UUID) ([]*ModelShare, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, model_id, owner_org_id, target_org_id, permission, created_at
		 FROM model_shares WHERE model_id = $1 ORDER BY created_at`, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []*ModelShare
	for rows.Next() {
		sh := &ModelShare{}
		if err := rows.Scan(&sh.ID, &sh.ModelID, &sh.OwnerOrgID, &sh.TargetOrgID, &sh.Permission, &sh.CreatedAt); err != nil {
			return nil, err
		}
		shares = append(shares, sh)
	}
	return shares, rows.Err()
}

// HasAccess checks if an org has access to a model (either owner or shared).
func (s *SharingService) HasAccess(ctx context.Context, modelID, orgID uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM model_shares WHERE model_id = $1 AND target_org_id = $2)`,
		modelID, orgID,
	).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return exists, err
}
