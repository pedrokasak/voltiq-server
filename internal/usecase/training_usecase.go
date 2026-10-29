package usecase

import (
	"context"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// TrainingUseCase handles the in-app training progress.
type TrainingUseCase struct {
	repo *repository.TrainingRepository
}

// NewTrainingUseCase creates a TrainingUseCase.
func NewTrainingUseCase(repo *repository.TrainingRepository) *TrainingUseCase {
	return &TrainingUseCase{repo: repo}
}

// TrainingStatus is the progress of the current user.
type TrainingStatus struct {
	Lessons     []string                       `json:"lessons"`
	Completed   []*repository.TrainingProgress `json:"completed"`
	ProgressPct float64                        `json:"progress_pct"`
	NextLesson  *string                        `json:"next_lesson"`
}

// TeamTraining is the progress of the tenant's users.
type TeamTraining struct {
	TotalLessons int                              `json:"total_lessons"`
	Members      []*repository.TeamMemberProgress `json:"members"`
	AvgPct       float64                          `json:"avg_pct"`
	GeneratedAt  time.Time                        `json:"generated_at"`
}

// Status returns the lessons completed by the user, the progress and the
// next lesson of the catalog.
func (uc *TrainingUseCase) Status(ctx context.Context, tenantID, userID domain.UUID) (*TrainingStatus, error) {
	done, err := uc.repo.List(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	completed := map[string]bool{}
	valid := []*repository.TrainingProgress{}
	for _, p := range done {
		if domain.IsTrainingLesson(p.LessonKey) {
			completed[p.LessonKey] = true
			valid = append(valid, p)
		}
	}
	s := &TrainingStatus{Lessons: domain.TrainingLessons, Completed: valid}
	s.ProgressPct = round2(float64(len(completed)) / float64(len(domain.TrainingLessons)) * 100)
	for _, k := range domain.TrainingLessons {
		if !completed[k] {
			next := k
			s.NextLesson = &next
			break
		}
	}
	return s, nil
}

func validateLesson(lesson string, score *int) error {
	v := &domain.ValidationError{}
	if !domain.IsTrainingLesson(lesson) {
		v.Fields = append(v.Fields, domain.FieldError{Field: "lesson_key", Message: "lição desconhecida"})
	}
	if score != nil && (*score < 0 || *score > 100) {
		v.Fields = append(v.Fields, domain.FieldError{Field: "quiz_score", Message: "nota deve estar entre 0 e 100"})
	}
	return v.OrNil()
}

// Complete marks a lesson as completed by the user.
func (uc *TrainingUseCase) Complete(ctx context.Context, tenantID, userID domain.UUID, lesson string, score *int) (*TrainingStatus, error) {
	if err := validateLesson(lesson, score); err != nil {
		return nil, err
	}
	if err := uc.repo.Complete(ctx, tenantID, userID, lesson, score); err != nil {
		return nil, err
	}
	return uc.Status(ctx, tenantID, userID)
}

// Reset marks a lesson as not completed.
func (uc *TrainingUseCase) Reset(ctx context.Context, tenantID, userID domain.UUID, lesson string) (*TrainingStatus, error) {
	if err := validateLesson(lesson, nil); err != nil {
		return nil, err
	}
	if err := uc.repo.Reset(ctx, tenantID, userID, lesson); err != nil {
		return nil, err
	}
	return uc.Status(ctx, tenantID, userID)
}

// Team returns the training progress of the tenant's users.
func (uc *TrainingUseCase) Team(ctx context.Context, tenantID domain.UUID) (*TeamTraining, error) {
	members, err := uc.repo.Team(ctx, tenantID, domain.TrainingLessons)
	if err != nil {
		return nil, err
	}
	t := &TeamTraining{TotalLessons: len(domain.TrainingLessons), Members: members, GeneratedAt: time.Now()}
	if len(members) > 0 {
		sum := 0
		for _, m := range members {
			sum += m.Completed
		}
		t.AvgPct = round2(float64(sum) / float64(len(members)*len(domain.TrainingLessons)) * 100)
	}
	return t, nil
}
