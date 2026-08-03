-- Migration 009: Billing Fase C - Dunning, Metrics, Webhook Events
-- Domain: Billing
-- Execute AFTER 008_billing_fase_b_payment.sql

-- 1. Webhook events table for idempotency (if not exists from 008)
CREATE TABLE IF NOT EXISTS payment_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway TEXT NOT NULL CHECK (gateway IN ('asaas','stripe','mercadopago')),
    event_id TEXT NOT NULL, -- ID do evento no gateway
    event_type TEXT NOT NULL, -- PAYMENT_RECEIVED, SUBSCRIPTION_DELETED, etc
    payload JSONB NOT NULL, -- Payload completo do webhook
    processed_at TIMESTAMPTZ,
    processing_error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(gateway, event_id)
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_gateway_event ON payment_webhook_events (gateway, event_id);
CREATE INDEX IF NOT EXISTS idx_webhook_events_processed ON payment_webhook_events (processed_at) WHERE processed_at IS NULL;

-- 2. Dunning events table for audit trail
CREATE TABLE IF NOT EXISTS billing_dunning_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    payment_gateway_id TEXT NOT NULL,
    stage INTEGER NOT NULL CHECK (stage IN (1,2,3)), -- 1=D+1, 2=D+7, 3=D+15
    email_sent_at TIMESTAMPTZ,
    email_template TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dunning_tenant ON billing_dunning_events (tenant_id);
CREATE INDEX IF NOT EXISTS idx_dunning_stage ON billing_dunning_events (stage);
CREATE INDEX IF NOT EXISTS idx_dunning_payment ON billing_dunning_events (payment_gateway_id);

-- 3. Daily billing metrics table (materialized)
CREATE TABLE IF NOT EXISTS billing_metrics_daily (
    date DATE PRIMARY KEY,
    mrr_cents BIGINT NOT NULL DEFAULT 0,
    arr_cents BIGINT NOT NULL DEFAULT 0,
    active_subscriptions INTEGER NOT NULL DEFAULT 0,
    churned_subscriptions INTEGER NOT NULL DEFAULT 0,
    new_subscriptions INTEGER NOT NULL DEFAULT 0,
    expansion_cents BIGINT NOT NULL DEFAULT 0,
    contraction_cents BIGINT NOT NULL DEFAULT 0,
    trial_conversions INTEGER NOT NULL DEFAULT 0,
    trial_expired INTEGER NOT NULL DEFAULT 0,
    by_plan JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 4. Trigger for updated_at on billing_metrics_daily
CREATE OR REPLACE FUNCTION update_billing_metrics_daily_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_billing_metrics_daily_updated_at ON billing_metrics_daily;
CREATE TRIGGER trigger_update_billing_metrics_daily_updated_at
BEFORE UPDATE ON billing_metrics_daily
FOR EACH ROW EXECUTE FUNCTION update_billing_metrics_daily_updated_at();

-- 5. Comments for documentation
COMMENT ON TABLE payment_webhook_events IS 'Webhook events for idempotent processing (gateway, event_id unique)';
COMMENT ON TABLE billing_dunning_events IS 'Dunning email audit trail (stages 1=D+1, 2=D+7, 3=D+15)';
COMMENT ON TABLE billing_metrics_daily IS 'Daily aggregated billing metrics (MRR, ARR, Churn, LTV, etc)';
COMMENT ON COLUMN billing_dunning_events.stage IS '1=D+1 first notice, 2=D+7 second notice, 3=D+15 final notice';