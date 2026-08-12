-- Migration 008: Billing Fase B - Payment Integration
-- Domain: Billing
-- Execute AFTER 007_billing_fase_a.sql

-- 1. Add payment integration columns to tenants table
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS payment_customer_id TEXT,
    ADD COLUMN IF NOT EXISTS payment_subscription_id TEXT,
    ADD COLUMN IF NOT EXISTS address TEXT,
    ADD COLUMN IF NOT EXISTS address_number TEXT,
    ADD COLUMN IF NOT EXISTS province TEXT,
    ADD COLUMN IF NOT EXISTS postal_code TEXT;

-- 2. Indexes for payment lookups
CREATE INDEX IF NOT EXISTS idx_tenants_payment_customer ON tenants (payment_customer_id) WHERE payment_customer_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tenants_payment_subscription ON tenants (payment_subscription_id) WHERE payment_subscription_id IS NOT NULL;

-- 3. Comments for documentation
COMMENT ON COLUMN tenants.payment_customer_id IS 'ID do cliente no gateway de pagamento (Asaas/Stripe/etc)';
COMMENT ON COLUMN tenants.payment_subscription_id IS 'ID da assinatura no gateway de pagamento';
COMMENT ON COLUMN tenants.address IS 'Endereço para cobrança';
COMMENT ON COLUMN tenants.address_number IS 'Número do endereço';
COMMENT ON COLUMN tenants.province IS 'Bairro/Província';
COMMENT ON COLUMN tenants.postal_code IS 'CEP para cobrança';

-- 4. Create payments table for local audit trail
CREATE TABLE IF NOT EXISTS tenant_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    payment_gateway_id TEXT NOT NULL, -- ID no Asaas/Stripe
    subscription_gateway_id TEXT, -- ID da assinatura no gateway
    amount_cents INTEGER NOT NULL, -- Valor em centavos (BRL)
    currency TEXT NOT NULL DEFAULT 'BRL',
    status TEXT NOT NULL CHECK (status IN ('PENDING','RECEIVED','OVERDUE','REFUNDED','CONFIRMED','DELETED','FAILED')),
    billing_type TEXT NOT NULL CHECK (billing_type IN ('BOLETO','CREDIT_CARD','PIX','UNDEFINED')),
    due_date DATE NOT NULL,
    paid_at TIMESTAMPTZ,
    invoice_url TEXT,
    bank_slip_url TEXT,
    pix_qr_code TEXT,
    pix_code TEXT,
    gateway_response JSONB, -- Resposta completa do gateway para debug
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for payments
CREATE INDEX IF NOT EXISTS idx_tenant_payments_tenant ON tenant_payments (tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_payments_gateway ON tenant_payments (payment_gateway_id);
CREATE INDEX IF NOT EXISTS idx_tenant_payments_subscription ON tenant_payments (subscription_gateway_id);
CREATE INDEX IF NOT EXISTS idx_tenant_payments_due_date ON tenant_payments (due_date);
CREATE INDEX IF NOT EXISTS idx_tenant_payments_status ON tenant_payments (status);

-- 5. Create webhook events table for idempotency
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

-- 6. Trigger to update updated_at on tenant_payments
CREATE OR REPLACE FUNCTION update_tenant_payments_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_tenant_payments_updated_at ON tenant_payments;
CREATE TRIGGER trigger_update_tenant_payments_updated_at
BEFORE UPDATE ON tenant_payments
FOR EACH ROW EXECUTE FUNCTION update_tenant_payments_updated_at();

-- 7. Update plan max_users defaults in tenants table
-- (Already set in migration 007, but ensuring consistency)
UPDATE tenants
SET max_users = CASE
    WHEN plan = 'trial' THEN 5
    WHEN plan = 'starter' THEN 10
    WHEN plan = 'pro' THEN 50
    WHEN plan = 'enterprise' THEN 999999
    ELSE 5
END
WHERE max_users IS NULL OR max_users = 0;