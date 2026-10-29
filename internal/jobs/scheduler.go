package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/reqctx"
	"github.com/voltiq/server/internal/usecase"
)

// Job is a named periodic task. Daily jobs run once a day at Hour:Minute
// (America/Sao_Paulo); interval jobs run every Every.
type Job struct {
	Name   string
	Key    int64 // advisory lock key, unique per job
	Every  time.Duration
	Hour   int
	Minute int
	Run    func(ctx context.Context) error
}

// Scheduler runs jobs in-process. Each run takes a Postgres advisory lock
// so only one machine executes it.
type Scheduler struct {
	db   *repository.Database
	jobs []Job
	loc  *time.Location
	wg   sync.WaitGroup
}

// NewScheduler creates a Scheduler.
func NewScheduler(db *repository.Database, jobs ...Job) *Scheduler {
	return &Scheduler{db: db, jobs: jobs, loc: domain.BusinessLocation}
}

// Start launches one goroutine per job; they stop when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	for _, j := range s.jobs {
		s.wg.Add(1)
		go func(j Job) {
			defer s.wg.Done()
			for {
				wait := s.next(j, time.Now())
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
					s.runOnce(ctx, j)
				}
			}
		}(j)
	}
	slog.Info("scheduler started", "jobs", len(s.jobs))
}

// Wait blocks until all job goroutines have stopped.
func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) next(j Job, now time.Time) time.Duration {
	if j.Every > 0 {
		return j.Every
	}
	local := now.In(s.loc)
	at := time.Date(local.Year(), local.Month(), local.Day(), j.Hour, j.Minute, 0, 0, s.loc)
	if !at.After(local) {
		at = at.AddDate(0, 0, 1)
	}
	return at.Sub(local)
}

// RunAll runs every job once (used by the "cron" command).
func (s *Scheduler) RunAll(ctx context.Context) {
	for _, j := range s.jobs {
		s.runOnce(ctx, j)
	}
}

func (s *Scheduler) runOnce(ctx context.Context, j Job) {
	began := time.Now()
	ran, err := s.db.WithAdvisoryLock(ctx, j.Key, j.Run)
	switch {
	case err != nil:
		slog.Error("job failed", "job", j.Name, "error", err, "duration_ms", time.Since(began).Milliseconds())
	case !ran:
		slog.Info("job skipped, running elsewhere", "job", j.Name)
	default:
		slog.Info("job completed", "job", j.Name, "duration_ms", time.Since(began).Milliseconds())
	}
}

// Deps are the use cases the standard jobs need.
type Deps struct {
	TenantRepo  *repository.TenantRepository
	BalanceRepo *repository.BalanceRepository
	Billing     *usecase.BillingUseCase
	Balance     *usecase.BalanceUseCase
	Alerts      *usecase.AlertUseCase
}

// StandardJobs returns the platform jobs (SDD 11-BILLING-ONBOARDING e
// calculo automatico diario):
//   - trial_expiration (diario 00:15)
//   - dunning (diario 09:05)
//   - billing_metrics (diario 00:40, dia anterior)
//   - webhook_retry (a cada 15 min)
//   - alert_retry (a cada 10 min)
//   - balance_daily (diario 02:10: mes corrente e, ate o dia 5, o anterior)
func StandardJobs(d Deps) []Job {
	var out []Job
	if d.TenantRepo != nil {
		out = append(out, Job{Name: "trial_expiration", Key: 70001, Hour: 0, Minute: 15,
			Run: NewTrialExpirationJob(d.TenantRepo).Run})
	}
	if d.Billing != nil {
		out = append(out,
			Job{Name: "dunning", Key: 70002, Hour: 9, Minute: 5, Run: d.Billing.RunDunningJob},
			Job{Name: "billing_metrics", Key: 70003, Hour: 0, Minute: 40, Run: func(ctx context.Context) error {
				return d.Billing.RunMetricsJob(ctx, time.Now().AddDate(0, 0, -1))
			}},
			Job{Name: "webhook_retry", Key: 70004, Every: 15 * time.Minute, Run: func(ctx context.Context) error {
				n, err := d.Billing.RetryFailedWebhooks(ctx)
				if n > 0 {
					slog.Info("webhooks reprocessed", "count", n)
				}
				return err
			}},
		)
	}
	if d.Alerts != nil && d.BalanceRepo != nil {
		out = append(out, Job{Name: "alert_retry", Key: 70005, Every: 10 * time.Minute, Run: func(ctx context.Context) error {
			n, err := d.Alerts.RetryFailed(ctx, d.BalanceRepo)
			if n > 0 {
				slog.Info("alerts retried", "count", n)
			}
			return err
		}})
	}
	if d.Balance != nil && d.TenantRepo != nil {
		out = append(out, Job{Name: "balance_daily", Key: 70006, Hour: 2, Minute: 10, Run: func(ctx context.Context) error {
			return RecalculateAll(ctx, d.TenantRepo, d.Balance, time.Now())
		}})
	}
	return out
}

// RecalculateAll recalcula o balanco do mes corrente de todos os tenants
// ativos ou em trial; ate o dia 5 tambem o mes anterior (leituras atrasadas).
func RecalculateAll(ctx context.Context, tenantRepo *repository.TenantRepository, balance *usecase.BalanceUseCase, now time.Time) error {
	tenants, err := tenantRepo.ListAll(ctx)
	if err != nil {
		return err
	}
	monthStart := domain.MonthStart(now)
	periods := [][2]time.Time{{monthStart, monthStart.AddDate(0, 1, 0).Add(-time.Nanosecond)}}
	if now.In(domain.BusinessLocation).Day() <= 5 {
		prev := monthStart.AddDate(0, -1, 0)
		periods = append(periods, [2]time.Time{prev, monthStart.Add(-time.Nanosecond)})
	}
	for _, t := range tenants {
		if t.Status != domain.TenantStatusActive && t.Status != domain.TenantStatusTrial {
			continue
		}
		// Contexto de sistema com o tenant: repositorios filtram e o pool
		// aplica as variaveis de RLS.
		tctx := reqctx.WithIdentity(ctx, reqctx.Identity{TenantID: t.ID, Email: "scheduler@voltiq"})
		for _, p := range periods {
			if _, err := balance.CalculateAll(tctx, t.ID, p[0], p[1]); err != nil {
				slog.Error("scheduled balance failed", "tenant_id", t.ID, "error", err)
			}
		}
	}
	return nil
}
