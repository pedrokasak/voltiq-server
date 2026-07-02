package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/energybalance/server/internal/domain"
)

// BalanceRepository handles transformer balance data access
type BalanceRepository struct {
	db *Database
}

// NewBalanceRepository creates a new BalanceRepository
func NewBalanceRepository(db *Database) *BalanceRepository {
	return &BalanceRepository{db: db}
}

// Create inserts a new transformer balance into the database
func (r *BalanceRepository) Create(ctx context.Context, balance *domain.TransformerBalance) error {
	query := `
		INSERT INTO transformer_balance (
			id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		balance.ID,
		balance.TenantID,
		balance.TransformerID,
		balance.PeriodStart,
		balance.PeriodEnd,
		balance.EnergyInjectedKWh,
		balance.TotalConsumptionKWh,
		balance.LossKWh,
		balance.LossPct,
		balance.TechnicalLossKWh,
		balance.NonTechnicalLossKWh,
		balance.Status,
		balance.UCCount,
		balance.CalculatedAt,
	)

	return err
}

// GetByTransformerAndPeriod retrieves balances for a transformer in a period
func (r *BalanceRepository) GetByTransformerAndPeriod(
	ctx context.Context,
	transformerID domain.UUID,
	start, end time.Time,
) ([]*domain.TransformerBalance, error) {
	query := `
		SELECT id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		FROM transformer_balance
		WHERE transformer_id = $1 AND period_start >= $2 AND period_start <= $3
		ORDER BY period_start DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, transformerID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var balances []*domain.TransformerBalance
	for rows.Next() {
		balance := &domain.TransformerBalance{}

		err := rows.Scan(
			&balance.ID,
			&balance.TenantID,
			&balance.TransformerID,
			&balance.PeriodStart,
			&balance.PeriodEnd,
			&balance.EnergyInjectedKWh,
			&balance.TotalConsumptionKWh,
			&balance.LossKWh,
			&balance.LossPct,
			&balance.TechnicalLossKWh,
			&balance.NonTechnicalLossKWh,
			&balance.Status,
			&balance.UCCount,
			&balance.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}

		balances = append(balances, balance)
	}

	return balances, nil
}

// GetLatestByTransformer retrieves the latest balance for a transformer
func (r *BalanceRepository) GetLatestByTransformer(
	ctx context.Context,
	transformerID domain.UUID,
) (*domain.TransformerBalance, error) {
	query := `
		SELECT id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		FROM transformer_balance
		WHERE transformer_id = $1
		ORDER BY period_start DESC
		LIMIT 1
	`

	balance := &domain.TransformerBalance{}

	err := r.db.Pool.QueryRow(ctx, query, transformerID).Scan(
		&balance.ID,
		&balance.TenantID,
		&balance.TransformerID,
		&balance.PeriodStart,
		&balance.PeriodEnd,
		&balance.EnergyInjectedKWh,
		&balance.TotalConsumptionKWh,
		&balance.LossKWh,
		&balance.LossPct,
		&balance.TechnicalLossKWh,
		&balance.NonTechnicalLossKWh,
		&balance.Status,
		&balance.UCCount,
		&balance.CalculatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return balance, nil
}

// GetByTenant retrieves balances for a tenant
func (r *BalanceRepository) GetByTenant(
	ctx context.Context,
	tenantID domain.UUID,
	limit int,
) ([]*domain.TransformerBalance, error) {
	query := `
		SELECT id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		FROM transformer_balance
		WHERE tenant_id = $1
		ORDER BY period_start DESC
		LIMIT $2
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var balances []*domain.TransformerBalance
	for rows.Next() {
		balance := &domain.TransformerBalance{}

		err := rows.Scan(
			&balance.ID,
			&balance.TenantID,
			&balance.TransformerID,
			&balance.PeriodStart,
			&balance.PeriodEnd,
			&balance.EnergyInjectedKWh,
			&balance.TotalConsumptionKWh,
			&balance.LossKWh,
			&balance.LossPct,
			&balance.TechnicalLossKWh,
			&balance.NonTechnicalLossKWh,
			&balance.Status,
			&balance.UCCount,
			&balance.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}

		balances = append(balances, balance)
	}

	return balances, nil
}
