package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/energybalance/server/internal/domain"
)

// InviteRepository handles invite data access
type InviteRepository struct {
	db *Database
}

// NewInviteRepository creates a new InviteRepository
func NewInviteRepository(db *Database) *InviteRepository {
	return &InviteRepository{db: db}
}

// Create inserts a new invite into the database
func (r *InviteRepository) Create(ctx context.Context, invite *domain.Invite) error {
	query := `
		INSERT INTO invites (id, tenant_id, email, role, token, status, invited_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		invite.ID,
		invite.TenantID,
		invite.Email,
		invite.Role,
		invite.Token,
		invite.Status,
		invite.InvitedBy,
		invite.ExpiresAt,
		invite.CreatedAt,
	)

	return err
}

// GetByToken retrieves an invite by token
func (r *InviteRepository) GetByToken(ctx context.Context, token string) (*domain.Invite, error) {
	query := `
		SELECT id, tenant_id, email, role, token, status, invited_by, accepted_at, expires_at, created_at, deleted_at
		FROM invites
		WHERE token = $1 AND deleted_at IS NULL
	`

	invite := &domain.Invite{}
	var acceptedAt pgtype.Timestamptz
	var deletedAt pgtype.Timestamptz

	err := r.db.Pool.QueryRow(ctx, query, token).Scan(
		&invite.ID,
		&invite.TenantID,
		&invite.Email,
		&invite.Role,
		&invite.Token,
		&invite.Status,
		&invite.InvitedBy,
		&acceptedAt,
		&invite.ExpiresAt,
		&invite.CreatedAt,
		&deletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if acceptedAt.Valid {
		invite.AcceptedAt = &acceptedAt.Time
	}
	if deletedAt.Valid {
		invite.DeletedAt = &deletedAt.Time
	}

	return invite, nil
}

// GetByEmail retrieves an invite by email
func (r *InviteRepository) GetByEmail(ctx context.Context, email string) (*domain.Invite, error) {
	query := `
		SELECT id, tenant_id, email, role, token, status, invited_by, accepted_at, expires_at, created_at, deleted_at
		FROM invites
		WHERE email = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	invite := &domain.Invite{}
	var acceptedAt pgtype.Timestamptz
	var deletedAt pgtype.Timestamptz

	err := r.db.Pool.QueryRow(ctx, query, email).Scan(
		&invite.ID,
		&invite.TenantID,
		&invite.Email,
		&invite.Role,
		&invite.Token,
		&invite.Status,
		&invite.InvitedBy,
		&acceptedAt,
		&invite.ExpiresAt,
		&invite.CreatedAt,
		&deletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if acceptedAt.Valid {
		invite.AcceptedAt = &acceptedAt.Time
	}
	if deletedAt.Valid {
		invite.DeletedAt = &deletedAt.Time
	}

	return invite, nil
}

// Update updates an existing invite
func (r *InviteRepository) Update(ctx context.Context, invite *domain.Invite) error {
	query := `
		UPDATE invites
		SET status = $1, accepted_at = $2, deleted_at = $3, updated_at = $4
		WHERE id = $5
	`

	var acceptedAt, deletedAt *time.Time
	if invite.AcceptedAt != nil {
		acceptedAt = invite.AcceptedAt
	}
	if invite.DeletedAt != nil {
		deletedAt = invite.DeletedAt
	}

	_, err := r.db.Pool.Exec(ctx, query,
		invite.Status,
		acceptedAt,
		deletedAt,
		time.Now(),
		invite.ID,
	)

	return err
}

// Delete performs a soft delete on an invite
func (r *InviteRepository) Delete(ctx context.Context, id domain.UUID) error {
	query := `
		UPDATE invites
		SET deleted_at = $1
		WHERE id = $2
	`

	_, err := r.db.Pool.Exec(ctx, query, time.Now(), id)
	return err
}
