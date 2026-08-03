-- Migration 010: Billing Fase C - Proration Credits
-- Domain: Billing
-- Execute AFTER 009_billing_fase_c_dunning_metrics.sql

-- 1. Proration credits table for mid-cycle plan changes
CREATE TABLE IF NOT EXISTS billing_proration_credits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subscription_gateway_id TEXT NOT NULL,
    amount_cents BIGINT NOT NULL, -- positivo = credito (downgrade), negativo = debito (upgrade)
    reason TEXT NOT NULL CHECK (reason IN ('upgrade','downgrade')),
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    days_calculated INTEGER NOT NULL,
    daily_rate_old_cents BIGINT NOT NULL,
    daily_rate_new_cents BIGINT NOT NULL,
    applied_at TIMESTAMPTZ, -- NULL = pendente, setado quando aplicado na fatura
    applied_payment_gateway_id TEXT, -- ID do payment no gateway onde foi aplicado
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_proration_tenant ON billing_proration_credits (tenant_id);
CREATE INDEX IF NOT EXISTS idx_proration_subscription ON billing_proration_credits (subscription_gateway_id);
CREATE INDEX IF NOT EXISTS idx_proration_applied ON billing_proration_credits (applied_at) WHERE applied_at IS NULL;

-- 2. Comments for documentation
COMMENT ON TABLE billing_proration_credits IS 'Proration credits/debits for mid-cycle plan changes (upgrade=debito, downgrade=credito)';
COMMENT ON COLUMN billing_proration_credits.amount_cents IS 'Positivo = credito para tenant (downgrade), Negativo = debito a cobrar (upgrade)';
COMMENT ON COLUMN billing_proration_credits.reason IS 'upgrade = cobrar diferenca proporcional agora; downgrade = credito no proximo ciclo';
COMMENT ON COLUMN billing_proration_credits.applied_at IS 'NULL = pendente aplicacao na proxima fatura; setado quando processado';
COMMENT ON COLUMN billing_proration_credits.days_calculated IS 'Dias restantes no ciclo atual no momento da mudanca';
COMMENT ON COLUMN billing_proration_credits.daily_rate_old_cents IS 'Valor diario do plano antigo (em centavos)';
COMMENT ON COLUMN billing_proration_credits.daily_rate_new_cents IS 'Valor diario do novo plano (em centavos)';