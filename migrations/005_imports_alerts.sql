-- Migration 005: CSV Imports and Alerts tables
-- Domain: Imports and Alerts

CREATE TABLE imports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    user_id     UUID REFERENCES users(id),
    file_name   TEXT NOT NULL,
    total_rows  INTEGER,
    rows_ok     INTEGER,
    rows_error  INTEGER,
    status      TEXT NOT NULL CHECK (status IN ('PROCESSING','COMPLETED','ERROR')),
    errors_json JSONB,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE alerts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    transformer_id UUID NOT NULL REFERENCES transformers(id),
    balance_id  UUID NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('WARNING','CRITICAL')),
    channel     TEXT NOT NULL CHECK (channel IN ('EMAIL','WHATSAPP')),
    recipient   TEXT NOT NULL,
    delivery_status TEXT NOT NULL CHECK (delivery_status IN ('PENDING','SENT','ERROR')),
    sent_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_imports_tenant ON imports (tenant_id);
CREATE INDEX idx_alerts_tenant ON alerts (tenant_id);

-- Enable Row-Level Security
ALTER TABLE imports ENABLE ROW LEVEL SECURITY;
ALTER TABLE alerts ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_imports ON imports
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

CREATE POLICY tenant_isolation_alerts ON alerts
    USING (tenant_id = current_setting('app.tenant_id')::UUID);
