-- Migration: 001_enable_rls_and_security
-- Description: Enable Row Level Security (RLS) and security policies for all tables
-- References: SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md, 05-SEGURANCA/01-autenticacao-autorizacao.md

-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- TENANTS
-- ============================================================================

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;

-- Policy: Tenants can only be accessed by users belonging to that tenant
CREATE POLICY tenant_isolation_policy ON tenants
    FOR ALL
    USING (id = current_setting('app.current_tenant_id', true)::UUID);

-- Policy: Only admins can modify tenants
CREATE POLICY tenant_admin_policy ON tenants
    FOR UPDATE
    USING (
        EXISTS (
            SELECT 1 FROM users
            WHERE users.tenant_id = tenants.id
            AND users.email = current_setting('app.current_user_email', true)
            AND users.role = 'ADMIN'
        )
    );

-- ============================================================================
-- USERS
-- ============================================================================

ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only be accessed by users from the same tenant
CREATE POLICY user_tenant_isolation_policy ON users
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- Policy: Users can view their own record
CREATE POLICY user_self_read_policy ON users
    FOR SELECT
    USING (email = current_setting('app.current_user_email', true));

-- Policy: Admins can manage all users in their tenant
CREATE POLICY user_admin_policy ON users
    FOR ALL
    USING (
        EXISTS (
            SELECT 1 FROM users AS admin_users
            WHERE admin_users.tenant_id = users.tenant_id
            AND admin_users.email = current_setting('app.current_user_email', true)
            AND admin_users.role = 'ADMIN'
        )
    );

-- ============================================================================
-- SUBESTACOES
-- ============================================================================

ALTER TABLE subestacoes ENABLE ROW LEVEL SECURITY;

CREATE POLICY substation_tenant_policy ON subestacoes
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- ============================================================================
-- TRAFOS (TRANSFORMERS)
-- ============================================================================

ALTER TABLE trafos ENABLE ROW LEVEL SECURITY;

CREATE POLICY transformer_tenant_policy ON trafos
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- ============================================================================
-- UNIDADES CONSUMIDORAS
-- ============================================================================

ALTER TABLE unidades_consumidoras ENABLE ROW LEVEL SECURITY;

CREATE POLICY uc_tenant_policy ON unidades_consumidoras
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- ============================================================================
-- LEITURAS_TRAFO (TRANSFORMER READINGS)
-- ============================================================================

ALTER TABLE leituras_trafo ENABLE ROW LEVEL SECURITY;

CREATE POLICY trafo_reading_tenant_policy ON leituras_trafo
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- ============================================================================
-- LEITURAS_UC (UC READINGS)
-- ============================================================================

ALTER TABLE leituras_uc ENABLE ROW LEVEL SECURITY;

CREATE POLICY uc_reading_tenant_policy ON leituras_uc
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- ============================================================================
-- BALANCO_TRAFO (TRANSFORMER BALANCE)
-- ============================================================================

ALTER TABLE balanco_trafo ENABLE ROW LEVEL SECURITY;

CREATE POLICY balance_tenant_policy ON balanco_trafo
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- ============================================================================
-- IMPORTACOES (IMPORTS)
-- ============================================================================

ALTER TABLE importacoes ENABLE ROW LEVEL SECURITY;

CREATE POLICY import_tenant_policy ON importacoes
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- Policy: Users can only see their own imports (or admins can see all)
CREATE POLICY import_user_policy ON importacoes
    FOR SELECT
    USING (
        user_id = current_setting('app.current_user_id', true)::UUID
        OR EXISTS (
            SELECT 1 FROM users
            WHERE users.tenant_id = importacoes.tenant_id
            AND users.email = current_setting('app.current_user_email', true)
            AND users.role = 'ADMIN'
        )
    );

-- ============================================================================
-- ALERTAS (ALERTS)
-- ============================================================================

ALTER TABLE alertas ENABLE ROW LEVEL SECURITY;

CREATE POLICY alert_tenant_policy ON alertas
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- ============================================================================
-- INVITES (CONVITES)
-- ============================================================================

ALTER TABLE invites ENABLE ROW LEVEL SECURITY;

CREATE POLICY invite_tenant_policy ON invites
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

-- Policy: Only admins can create/modify invites
CREATE POLICY invite_admin_policy ON invites
    FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1 FROM users
            WHERE users.tenant_id = invites.tenant_id
            AND users.email = current_setting('app.current_user_email', true)
            AND users.role = 'ADMIN'
        )
    );

CREATE POLICY invite_admin_update_policy ON invites
    FOR UPDATE
    USING (
        EXISTS (
            SELECT 1 FROM users
            WHERE users.tenant_id = invites.tenant_id
            AND users.email = current_setting('app.current_user_email', true)
            AND users.role = 'ADMIN'
        )
    );

-- Policy: Users can view their own pending invites
CREATE POLICY invite_self_read_policy ON invites
    FOR SELECT
    USING (
        email = current_setting('app.current_user_email', true)
        AND status = 'PENDING'
    );

-- ============================================================================
-- BYPASS RLS FOR SUPERUSER
-- ============================================================================

-- Allow database superuser to bypass RLS
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
ALTER TABLE subestacoes FORCE ROW LEVEL SECURITY;
ALTER TABLE trafos FORCE ROW LEVEL SECURITY;
ALTER TABLE unidades_consumidoras FORCE ROW LEVEL SECURITY;
ALTER TABLE leituras_trafo FORCE ROW LEVEL SECURITY;
ALTER TABLE leituras_uc FORCE ROW LEVEL SECURITY;
ALTER TABLE balanco_trafo FORCE ROW LEVEL SECURITY;
ALTER TABLE importacoes FORCE ROW LEVEL SECURITY;
ALTER TABLE alertas FORCE ROW LEVEL SECURITY;
ALTER TABLE invites FORCE ROW LEVEL SECURITY;

-- ============================================================================
-- INDEXES FOR PERFORMANCE
-- ============================================================================

-- Index for tenant isolation checks
CREATE INDEX IF NOT EXISTS idx_users_tenant_email ON users(tenant_id, email);
CREATE INDEX IF NOT EXISTS idx_users_tenant_role ON users(tenant_id, role);

-- ============================================================================
-- AUDIT TRIGGER FUNCTION
-- ============================================================================

CREATE OR REPLACE FUNCTION audit_trigger_function()
RETURNS TRIGGER AS $$
BEGIN
    -- Log update/delete operations
    IF (TG_OP = 'UPDATE') THEN
        INSERT INTO audit_log (
            table_name, operation, old_data, new_data,
            user_email, tenant_id, created_at
        ) VALUES (
            TG_TABLE_NAME, TG_OP, to_jsonb(OLD), to_jsonb(NEW),
            current_setting('app.current_user_email', true),
            current_setting('app.current_tenant_id', true)::UUID,
            NOW()
        );
        RETURN NEW;
    ELSIF (TG_OP = 'DELETE') THEN
        INSERT INTO audit_log (
            table_name, operation, old_data,
            user_email, tenant_id, created_at
        ) VALUES (
            TG_TABLE_NAME, TG_OP, to_jsonb(OLD),
            current_setting('app.current_user_email', true),
            current_setting('app.current_tenant_id', true)::UUID,
            NOW()
        );
        RETURN OLD;
    ELSIF (TG_OP = 'INSERT') THEN
        INSERT INTO audit_log (
            table_name, operation, new_data,
            user_email, tenant_id, created_at
        ) VALUES (
            TG_TABLE_NAME, TG_OP, to_jsonb(NEW),
            current_setting('app.current_user_email', true),
            current_setting('app.current_tenant_id', true)::UUID,
            NOW()
        );
        RETURN NEW;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- ============================================================================
-- AUDIT LOG TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    table_name TEXT NOT NULL,
    operation TEXT NOT NULL,
    old_data JSONB,
    new_data JSONB,
    user_email TEXT,
    tenant_id UUID,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Enable RLS on audit log
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;

-- Only admins can view audit logs
CREATE POLICY audit_admin_policy ON audit_log
    FOR SELECT
    USING (
        EXISTS (
            SELECT 1 FROM users
            WHERE users.tenant_id = audit_log.tenant_id
            AND users.email = current_setting('app.current_user_email', true)
            AND users.role = 'ADMIN'
        )
    );

-- Create indexes for audit log queries
CREATE INDEX idx_audit_log_tenant ON audit_log(tenant_id, created_at DESC);
CREATE INDEX idx_audit_log_table ON audit_log(table_name, created_at DESC);
CREATE INDEX idx_audit_log_user ON audit_log(user_email, created_at DESC);
