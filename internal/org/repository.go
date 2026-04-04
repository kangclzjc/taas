package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Organization represents a TaaS organization.
type Organization struct {
	ID          uuid.UUID `json:"id"`
	Slug        string    `json:"slug"`
	DisplayName string    `json:"display_name"`
	SLATier     string    `json:"sla_tier"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// OrgMember represents a member of an organization.
type OrgMember struct {
	UserID   uuid.UUID `json:"user_id"`
	OrgID    uuid.UUID `json:"org_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// OrgRepository defines the interface for organization persistence operations.
type OrgRepository interface {
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id uuid.UUID) (*Organization, error)
	GetBySlug(ctx context.Context, slug string) (*Organization, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*Organization, error)
	ListMembers(ctx context.Context, orgID uuid.UUID) ([]*OrgMember, error)
	AddMember(ctx context.Context, member *OrgMember) error
	RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error
	UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, role string) error
}

// Repository handles organization persistence in PostgreSQL.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new org repository backed by PostgreSQL.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create inserts a new organization.
func (r *Repository) Create(ctx context.Context, org *Organization) error {
	if org.ID == uuid.Nil {
		org.ID = uuid.New()
	}
	now := time.Now().UTC()
	org.CreatedAt = now
	org.UpdatedAt = now
	if org.SLATier == "" {
		org.SLATier = "standard"
	}

	_, err := r.db.Exec(ctx,
		`INSERT INTO organizations (id, slug, display_name, sla_tier, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		org.ID, org.Slug, org.DisplayName, org.SLATier, org.CreatedAt, org.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting organization: %w", err)
	}
	return nil
}

// GetByID retrieves an organization by its ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Organization, error) {
	o := &Organization{}
	err := r.db.QueryRow(ctx,
		`SELECT id, slug, display_name, sla_tier, created_at, updated_at
		 FROM organizations WHERE id = $1`, id,
	).Scan(&o.ID, &o.Slug, &o.DisplayName, &o.SLATier, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying organization by id: %w", err)
	}
	return o, nil
}

// GetBySlug retrieves an organization by its slug.
func (r *Repository) GetBySlug(ctx context.Context, slug string) (*Organization, error) {
	o := &Organization{}
	err := r.db.QueryRow(ctx,
		`SELECT id, slug, display_name, sla_tier, created_at, updated_at
		 FROM organizations WHERE slug = $1`, slug,
	).Scan(&o.ID, &o.Slug, &o.DisplayName, &o.SLATier, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying organization by slug: %w", err)
	}
	return o, nil
}

// Update modifies an existing organization.
func (r *Repository) Update(ctx context.Context, org *Organization) error {
	org.UpdatedAt = time.Now().UTC()
	tag, err := r.db.Exec(ctx,
		`UPDATE organizations SET slug = $1, display_name = $2, sla_tier = $3, updated_at = $4
		 WHERE id = $5`,
		org.Slug, org.DisplayName, org.SLATier, org.UpdatedAt, org.ID,
	)
	if err != nil {
		return fmt.Errorf("updating organization: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("organization not found")
	}
	return nil
}

// Delete removes an organization by ID.
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting organization: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("organization not found")
	}
	return nil
}

// ListByUser returns all organizations a user belongs to.
func (r *Repository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*Organization, error) {
	rows, err := r.db.Query(ctx,
		`SELECT o.id, o.slug, o.display_name, o.sla_tier, o.created_at, o.updated_at
		 FROM organizations o
		 INNER JOIN org_members om ON o.id = om.org_id
		 WHERE om.user_id = $1
		 ORDER BY o.display_name`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing organizations by user: %w", err)
	}
	defer rows.Close()

	var orgs []*Organization
	for rows.Next() {
		o := &Organization{}
		if err := rows.Scan(&o.ID, &o.Slug, &o.DisplayName, &o.SLATier, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning organization: %w", err)
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

// ListMembers returns all members of an organization.
func (r *Repository) ListMembers(ctx context.Context, orgID uuid.UUID) ([]*OrgMember, error) {
	rows, err := r.db.Query(ctx,
		`SELECT org_id, user_id, role, joined_at
		 FROM org_members WHERE org_id = $1
		 ORDER BY joined_at`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing org members: %w", err)
	}
	defer rows.Close()

	var members []*OrgMember
	for rows.Next() {
		m := &OrgMember{}
		if err := rows.Scan(&m.OrgID, &m.UserID, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("scanning org member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// AddMember adds a user to an organization.
func (r *Repository) AddMember(ctx context.Context, member *OrgMember) error {
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now().UTC()
	}
	if member.Role == "" {
		member.Role = "member"
	}

	_, err := r.db.Exec(ctx,
		`INSERT INTO org_members (org_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (org_id, user_id) DO NOTHING`,
		member.OrgID, member.UserID, member.Role, member.JoinedAt,
	)
	if err != nil {
		return fmt.Errorf("adding org member: %w", err)
	}
	return nil
}

// RemoveMember removes a user from an organization.
func (r *Repository) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM org_members WHERE org_id = $1 AND user_id = $2`,
		orgID, userID,
	)
	if err != nil {
		return fmt.Errorf("removing org member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

// UpdateMemberRole updates the role of a member within an organization.
func (r *Repository) UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE org_members SET role = $1 WHERE org_id = $2 AND user_id = $3`,
		role, orgID, userID,
	)
	if err != nil {
		return fmt.Errorf("updating member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}
