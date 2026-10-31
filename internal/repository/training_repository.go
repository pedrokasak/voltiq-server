package repository

import (
	"context"
	"time"

	"github.com/voltiq/server/internal/domain"
)

// TrainingRepository stores the in-app training progress of the users.
type TrainingRepository struct {
	db *Database
}

// NewTrainingRepository creates a TrainingRepository.
func NewTrainingRepository(db *Database) *TrainingRepository {
	return &TrainingRepository{db: db}
}

// TrainingProgress is one completed lesson.
type TrainingProgress struct {
	LessonKey   string    `json:"lesson_key"`
	QuizScore   *int      `json:"quiz_score"`
	CompletedAt time.Time `json:"completed_at"`
}

// TeamMemberProgress is the training summary of one user of the tenant.
type TeamMemberProgress struct {
	UserID          domain.UUID `json:"user_id"`
	Name            string      `json:"name"`
	Email           string      `json:"email"`
	Role            string      `json:"role"`
	Completed       int         `json:"completed"`
	LastCompletedAt *time.Time  `json:"last_completed_at"`
}

// List returns the lessons completed by the user.
func (r *TrainingRepository) List(ctx context.Context, tenantID, userID domain.UUID) ([]*TrainingProgress, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT lesson_key, quiz_score, completed_at FROM training_progress
		WHERE tenant_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY completed_at`, tenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TrainingProgress{}
	for rows.Next() {
		p := &TrainingProgress{}
		if err := rows.Scan(&p.LessonKey, &p.QuizScore, &p.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Complete records the lesson as completed (keeps the best quiz score).
func (r *TrainingRepository) Complete(ctx context.Context, tenantID, userID domain.UUID, lesson string, score *int) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO training_progress (tenant_id, user_id, lesson_key, quiz_score)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, lesson_key) WHERE deleted_at IS NULL
		DO UPDATE SET quiz_score = GREATEST(training_progress.quiz_score, EXCLUDED.quiz_score),
			completed_at = NOW()`, tenantID, userID, lesson, score)
	return err
}

// Reset marks the lesson as not completed (soft delete).
func (r *TrainingRepository) Reset(ctx context.Context, tenantID, userID domain.UUID, lesson string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE training_progress SET deleted_at = NOW()
		WHERE tenant_id = $1 AND user_id = $2 AND lesson_key = $3 AND deleted_at IS NULL`, tenantID, userID, lesson)
	return err
}

// Team returns the progress of every active user of the tenant.
func (r *TrainingRepository) Team(ctx context.Context, tenantID domain.UUID, lessons []string) ([]*TeamMemberProgress, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT u.id, u.name, u.email, u.role,
			COUNT(p.id) FILTER (WHERE p.lesson_key = ANY($2)), MAX(p.completed_at)
		FROM users u
		LEFT JOIN training_progress p ON p.user_id = u.id AND p.tenant_id = u.tenant_id AND p.deleted_at IS NULL
		WHERE u.tenant_id = $1 AND u.active = true AND u.deleted_at IS NULL
		GROUP BY u.id, u.name, u.email, u.role
		ORDER BY u.name`, tenantID, lessons)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TeamMemberProgress{}
	for rows.Next() {
		m := &TeamMemberProgress{}
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role, &m.Completed, &m.LastCompletedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
