-- Migration 002: Electrical Network tables
-- Domain: Electrical Network

CREATE TABLE substations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    code        TEXT NOT NULL,
    name        TEXT NOT NULL,
    lat         DECIMAL(10,7),
    lng         DECIMAL(10,7),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ,
    UNIQUE (tenant_id, code)
);

CREATE TABLE transformers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id),
    substation_id       UUID REFERENCES substations(id),
    code                TEXT NOT NULL,
    power_kva           DECIMAL(10,2) NOT NULL,
    primary_voltage_kv  DECIMAL(6,2) NOT NULL,
    secondary_voltage_v DECIMAL(6,2) NOT NULL,
    lat                 DECIMAL(10,7),
    lng                 DECIMAL(10,7),
    -- Parameters for PRODIST M7 calculation
    core_loss_kw        DECIMAL(8,4),   -- P0: no-load losses (core losses)
    winding_loss_kw     DECIMAL(8,4),   -- Pcc: load losses (winding losses)
    loss_limit_pct      DECIMAL(5,2) DEFAULT 10.0, -- Regulatory limit ANEEL (%)
    active              BOOLEAN DEFAULT true,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ,
    deleted_at          TIMESTAMPTZ,
    UNIQUE (tenant_id, code)
);

CREATE TABLE consuming_units (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    transformer_id UUID NOT NULL REFERENCES transformers(id),
    uc_code     TEXT NOT NULL,
    name        TEXT,
    class       TEXT CHECK (class IN ('RESIDENTIAL','COMMERCIAL','INDUSTRIAL','RURAL','PUBLIC_POWER')),
    active      BOOLEAN DEFAULT true,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ,
    UNIQUE (tenant_id, uc_code)
);

CREATE INDEX idx_transformers_tenant ON transformers (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_consuming_units_transformer ON consuming_units (transformer_id) WHERE deleted_at IS NULL;

-- Enable Row-Level Security
ALTER TABLE substations ENABLE ROW LEVEL SECURITY;
ALTER TABLE transformers ENABLE ROW LEVEL SECURITY;
ALTER TABLE consuming_units ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_substations ON substations
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

CREATE POLICY tenant_isolation_transformers ON transformers
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

CREATE POLICY tenant_isolation_consuming_units ON consuming_units
    USING (tenant_id = current_setting('app.tenant_id')::UUID);
