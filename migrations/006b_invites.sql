-- Migration 006b: Invites table
-- Domain: Multitenancy / User management
-- Execute AFTER 006_alerts_soft_delete.sql, BEFORE 007_billing_fase_a.sql
-- (007's seat_count trigger references this table)

CREATE TABLE IF NOT EXISTS invites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    email       TEXT NOT NULL,
    role        TEXT NOT NULL
                    CHECK (role IN ('SUPER_ADMIN','OWNER','ADMIN','MANAGER','ENGINEER','VIEWER')),
    token       TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'PENDING'
                    CHECK (status IN ('PENDING','ACCEPTED','CANCELLED')),
    invited_by  UUID NOT NULL REFERENCES users(id),
    accepted_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_invites_tenant ON invites (tenant_id);
CREATE INDEX IF NOT EXISTS idx_invites_email ON invites (email);
CREATE INDEX IF NOT EXISTS idx_invites_token ON invites (token);

ALTER TABLE invites ENABLE ROW LEVEL SECURITY;

CREATE POLICY invite_tenant_isolation_policy ON invites
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);
