package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/voltiq/server/internal/domain"
)

// UserRepository handles user data access
type UserRepository struct {
	db *Database
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *Database) *UserRepository {
	return &UserRepository{db: db}
}

// Create inserts a new user into the database
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (id, tenant_id, email, name, password_hash, role, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		user.ID,
		user.TenantID,
		user.Email,
		user.Name,
		user.PasswordHash,
		user.Role,
		user.Active,
		user.CreatedAt,
		user.UpdatedAt,
	)

	return err
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.User, error) {
	query := `
		SELECT id, tenant_id, email, name, password_hash, role, active, last_login, created_at, updated_at, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`

	user := &domain.User{}
	var lastLogin pgtype.Timestamptz
	var deletedAt pgtype.Timestamptz

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.TenantID,
		&user.Email,
		&user.Name,
		&user.PasswordHash,
		&user.Role,
		&user.Active,
		&lastLogin,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}
	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}

	return user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, tenant_id, email, name, password_hash, role, active, last_login, created_at, updated_at, deleted_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`

	user := &domain.User{}
	var lastLogin pgtype.Timestamptz
	var deletedAt pgtype.Timestamptz

	err := r.db.Pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.TenantID,
		&user.Email,
		&user.Name,
		&user.PasswordHash,
		&user.Role,
		&user.Active,
		&lastLogin,
		&user.CreatedAt,
		&user.UpdatedAt,
		&deletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}
	if deletedAt.Valid {
		user.DeletedAt = &deletedAt.Time
	}

	return user, nil
}

// GetByTenant retrieves all users for a tenant
func (r *UserRepository) GetByTenant(ctx context.Context, tenantID domain.UUID) ([]*domain.User, error) {
	query := `
		SELECT id, tenant_id, email, name, password_hash, role, active, last_login, created_at, updated_at, deleted_at
		FROM users
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		user := &domain.User{}
		var lastLogin pgtype.Timestamptz
		var deletedAt pgtype.Timestamptz

		err := rows.Scan(
			&user.ID,
			&user.TenantID,
			&user.Email,
			&user.Name,
			&user.PasswordHash,
			&user.Role,
			&user.Active,
			&lastLogin,
			&user.CreatedAt,
			&user.UpdatedAt,
			&deletedAt,
		)
		if err != nil {
			return nil, err
		}

		if lastLogin.Valid {
			user.LastLogin = &lastLogin.Time
		}
		if deletedAt.Valid {
			user.DeletedAt = &deletedAt.Time
		}

		users = append(users, user)
	}

	return users, nil
}

// Update updates an existing user
func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users
		SET email = $1, name = $2, password_hash = $3, role = $4, active = $5, last_login = $6, updated_at = $7
		WHERE id = $8
	`

	var lastLogin *time.Time
	if user.LastLogin != nil {
		lastLogin = user.LastLogin
	}

	_, err := r.db.Pool.Exec(ctx, query,
		user.Email,
		user.Name,
		user.PasswordHash,
		user.Role,
		user.Active,
		lastLogin,
		time.Now(),
		user.ID,
	)

	return err
}

// Delete soft deletes a user
func (r *UserRepository) Delete(ctx context.Context, id domain.UUID) error {
	query := `
		UPDATE users
		SET deleted_at = $1, updated_at = $2
		WHERE id = $3
	`

	_, err := r.db.Pool.Exec(ctx, query, time.Now(), time.Now(), id)
	return err
}
