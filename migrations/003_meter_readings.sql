-- Migration 003: Meter Readings (Time Series) tables
-- Domain: Meter Readings with TimescaleDB hypertables

CREATE TABLE transformer_readings (
    id          UUID DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    transformer_id UUID NOT NULL REFERENCES transformers(id),
    reading_at  TIMESTAMPTZ NOT NULL,
    energy_kwh  DECIMAL(12,4) NOT NULL,
    demand_kw   DECIMAL(10,4),
    power_factor DECIMAL(4,3),
    import_id   UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, reading_at)
);

CREATE TABLE consuming_unit_readings (
    id          UUID DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    uc_id       UUID NOT NULL REFERENCES consuming_units(id),
    transformer_id UUID NOT NULL,
    reading_at  TIMESTAMPTZ NOT NULL,
    consumption_kwh DECIMAL(12,4) NOT NULL,
    import_id   UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, reading_at)
);

-- Create TimescaleDB hypertables
SELECT create_hypertable('transformer_readings', 'reading_at');
SELECT create_hypertable('consuming_unit_readings', 'reading_at');

-- Create indexes for performance
CREATE INDEX idx_transformer_readings_tenant_period ON transformer_readings (tenant_id, transformer_id, reading_at DESC);
CREATE INDEX idx_consuming_unit_readings_transformer_period ON consuming_unit_readings (transformer_id, reading_at DESC);

-- Enable compression after 3 months
SELECT add_compression_policy('transformer_readings', INTERVAL '3 months');
SELECT add_compression_policy('consuming_unit_readings', INTERVAL '3 months');

-- Enable Row-Level Security
ALTER TABLE transformer_readings ENABLE ROW LEVEL SECURITY;
ALTER TABLE consuming_unit_readings ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_transformer_readings ON transformer_readings
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

CREATE POLICY tenant_isolation_consuming_unit_readings ON consuming_unit_readings
    USING (tenant_id = current_setting('app.tenant_id')::UUID);
