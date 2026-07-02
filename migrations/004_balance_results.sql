-- Migration 004: Balance Calculation Results table
-- Domain: Calculation Results

CREATE TABLE transformer_balance (
    id              UUID DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    transformer_id  UUID NOT NULL REFERENCES transformers(id),
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    energy_injected_kwh     DECIMAL(14,4) NOT NULL,
    total_consumption_kwh   DECIMAL(14,4) NOT NULL,
    loss_kwh                DECIMAL(14,4) NOT NULL,
    loss_pct                DECIMAL(6,4) NOT NULL,
    technical_loss_kwh      DECIMAL(14,4),
    non_technical_loss_kwh  DECIMAL(14,4),
    status          TEXT NOT NULL CHECK (status IN ('NORMAL','WARNING','CRITICAL')),
    uc_count        INTEGER NOT NULL,
    calculated_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, period_start)
);

-- Create TimescaleDB hypertable
SELECT create_hypertable('transformer_balance', 'period_start');

-- Create index for performance
CREATE INDEX idx_transformer_balance_tenant ON transformer_balance (tenant_id, transformer_id, period_start DESC);

-- Enable Row-Level Security
ALTER TABLE transformer_balance ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_transformer_balance ON transformer_balance
    USING (tenant_id = current_setting('app.tenant_id')::UUID);
