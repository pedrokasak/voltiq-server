-- Migration 011: Fechamento das Sprints 1, 2 e 3
-- Domains: security (RLS), alerts, imports, audit, tenant settings, performance
--
-- Nao altera as migracoes 001-010: apenas recria politicas, adiciona colunas,
-- tabelas, triggers e indices. Reversao em 011_sprints_completion.down.sql.

-- ============================================================================
-- 1. RLS: uma unica funcao de tenant corrente
-- ============================================================================
-- As migracoes anteriores usam app.tenant_id (002-006) e app.current_tenant_id
-- (001b, 006b) e chamam current_setting sem missing_ok, o que quebra consultas
-- feitas sem contexto de tenant (login, signup, webhooks, jobs) quando a role
-- do banco nao tem BYPASSRLS. A API agora define as duas variaveis por conexao
-- (internal/repository/database.go). Sem tenant na sessao, a politica libera
-- o acesso de sistema; com tenant, restringe ao tenant da sessao.

CREATE OR REPLACE FUNCTION app_current_tenant() RETURNS UUID
LANGUAGE sql STABLE AS $$
    SELECT NULLIF(COALESCE(
        NULLIF(current_setting('app.tenant_id', true), ''),
        current_setting('app.current_tenant_id', true)
    ), '')::UUID
$$;

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'substations', 'transformers', 'consuming_units',
        'transformer_readings', 'consuming_unit_readings',
        'transformer_balance', 'imports', 'alerts', 'invites'
    ] LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_%s ON %I', t, t);
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation_%s ON %I USING (app_current_tenant() IS NULL OR tenant_id = app_current_tenant()) WITH CHECK (app_current_tenant() IS NULL OR tenant_id = app_current_tenant())',
            t, t);
    END LOOP;
END $$;

DROP POLICY IF EXISTS invite_tenant_isolation_policy ON invites;

-- tenants: a linha do proprio tenant
DROP POLICY IF EXISTS tenant_isolation_tenants ON tenants;
DROP POLICY IF EXISTS tenant_isolation_policy ON tenants;
DROP POLICY IF EXISTS tenant_isolation_policy_update ON tenants;
DROP POLICY IF EXISTS tenant_isolation_policy_delete ON tenants;
CREATE POLICY tenant_isolation_tenants ON tenants
    USING (app_current_tenant() IS NULL OR id = app_current_tenant())
    WITH CHECK (app_current_tenant() IS NULL OR id = app_current_tenant());

-- users: usuarios do proprio tenant
DROP POLICY IF EXISTS tenant_isolation_users ON users;
DROP POLICY IF EXISTS user_tenant_isolation_policy ON users;
CREATE POLICY tenant_isolation_users ON users
    USING (app_current_tenant() IS NULL OR tenant_id = app_current_tenant())
    WITH CHECK (app_current_tenant() IS NULL OR tenant_id = app_current_tenant());

-- audit_log: leitura pelo tenant (a API restringe a ADMIN/OWNER)
DROP POLICY IF EXISTS audit_admin_policy ON audit_log;
DROP POLICY IF EXISTS tenant_isolation_audit_log ON audit_log;
CREATE POLICY tenant_isolation_audit_log ON audit_log
    USING (app_current_tenant() IS NULL OR tenant_id = app_current_tenant());

-- ============================================================================
-- 2. Alertas: destinatarios (RF05.3), grupos de trafos e historico (RF05.4)
-- ============================================================================

CREATE TABLE IF NOT EXISTS transformer_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_transformer_groups_tenant_name
    ON transformer_groups (tenant_id, name) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS transformer_group_members (
    group_id       UUID NOT NULL REFERENCES transformer_groups(id),
    transformer_id UUID NOT NULL REFERENCES transformers(id),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (group_id, transformer_id)
);
CREATE INDEX IF NOT EXISTS idx_transformer_group_members_transformer
    ON transformer_group_members (transformer_id);

-- Destinatario: por trafo, por grupo ou para todos os trafos do tenant
-- (transformer_id e group_id nulos). min_level define a partir de qual
-- status o alerta e enviado.
CREATE TABLE IF NOT EXISTS alert_recipients (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    transformer_id UUID REFERENCES transformers(id),
    group_id       UUID REFERENCES transformer_groups(id),
    channel        TEXT NOT NULL CHECK (channel IN ('EMAIL','WHATSAPP')),
    recipient      TEXT NOT NULL,
    min_level      TEXT NOT NULL DEFAULT 'WARNING' CHECK (min_level IN ('WARNING','CRITICAL')),
    active         BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ,
    deleted_at     TIMESTAMPTZ,
    CHECK (transformer_id IS NULL OR group_id IS NULL)
);
CREATE INDEX IF NOT EXISTS idx_alert_recipients_tenant
    ON alert_recipients (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_alert_recipients_transformer
    ON alert_recipients (transformer_id) WHERE deleted_at IS NULL;

-- alerts passa a ser o historico de envios
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS recipient_id UUID REFERENCES alert_recipients(id);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS loss_pct DECIMAL(6,2);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS limit_pct DECIMAL(6,2);
CREATE INDEX IF NOT EXISTS idx_alerts_tenant_created ON alerts (tenant_id, created_at DESC);

-- ============================================================================
-- 3. Importacoes: reprocessamento, mapeamento de colunas e desfazer
-- ============================================================================

ALTER TABLE imports ADD COLUMN IF NOT EXISTS reading_type TEXT;
ALTER TABLE imports ADD COLUMN IF NOT EXISTS file_content BYTEA;
ALTER TABLE imports ADD COLUMN IF NOT EXISTS column_mapping JSONB;
ALTER TABLE imports ADD COLUMN IF NOT EXISTS duplicates_skipped INTEGER NOT NULL DEFAULT 0;
ALTER TABLE imports ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
ALTER TABLE imports ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_imports_tenant_created ON imports (tenant_id, created_at DESC) WHERE deleted_at IS NULL;

-- ============================================================================
-- 4. Configuracoes do tenant (tarifa do dashboard financeiro e onboarding)
-- ============================================================================

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS tariff_brl_mwh DECIMAL(10,2);
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS onboarding_completed_at TIMESTAMPTZ;

-- ============================================================================
-- 5. Auditoria de acoes criticas (RF07.4, RNF03.7)
-- ============================================================================
-- A funcao da 001b converte current_tenant_id direto para UUID e falha com
-- valor vazio; esta versao trata vazio como nulo e remove campos sensiveis
-- ou volumosos (senha, conteudo de arquivo).

CREATE OR REPLACE FUNCTION audit_row_change() RETURNS TRIGGER
LANGUAGE plpgsql SECURITY DEFINER AS $$
DECLARE
    v_tenant UUID;
    v_old JSONB;
    v_new JSONB;
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        v_old := to_jsonb(OLD) - 'password_hash' - 'file_content';
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        v_new := to_jsonb(NEW) - 'password_hash' - 'file_content';
    END IF;

    v_tenant := COALESCE(
        app_current_tenant(),
        NULLIF(COALESCE(v_new->>'tenant_id', v_old->>'tenant_id'), '')::UUID
    );
    IF TG_TABLE_NAME = 'tenants' THEN
        v_tenant := COALESCE(NULLIF(COALESCE(v_new->>'id', v_old->>'id'), '')::UUID, v_tenant);
    END IF;

    -- UPDATE sem mudanca de conteudo (ex.: so updated_at) nao gera registro
    IF TG_OP = 'UPDATE' AND (v_old - 'updated_at' - 'last_login') = (v_new - 'updated_at' - 'last_login') THEN
        RETURN NEW;
    END IF;

    INSERT INTO audit_log (table_name, operation, old_data, new_data, user_email, tenant_id, created_at)
    VALUES (
        TG_TABLE_NAME, TG_OP, v_old, v_new,
        NULLIF(current_setting('app.current_user_email', true), ''),
        v_tenant, NOW()
    );

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'transformers', 'consuming_units', 'substations', 'imports',
        'users', 'invites', 'alert_recipients', 'transformer_groups'
    ] LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS audit_%s ON %I', t, t);
        EXECUTE format(
            'CREATE TRIGGER audit_%s AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION audit_row_change()',
            t, t);
    END LOOP;
END $$;

DROP TRIGGER IF EXISTS audit_tenants ON tenants;
CREATE TRIGGER audit_tenants AFTER UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION audit_row_change();

-- ============================================================================
-- 6. RLS nas tabelas novas
-- ============================================================================

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['transformer_groups', 'transformer_group_members', 'alert_recipients'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_%s ON %I', t, t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation_%s ON %I USING (app_current_tenant() IS NULL OR tenant_id = app_current_tenant()) WITH CHECK (app_current_tenant() IS NULL OR tenant_id = app_current_tenant())',
            t, t);
    END LOOP;
END $$;

-- ============================================================================
-- 7. Desempenho para 50 mil transformadores (RNF04.3)
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_transformer_readings_transformer_time
    ON transformer_readings (transformer_id, reading_at);
CREATE INDEX IF NOT EXISTS idx_consuming_unit_readings_uc_time
    ON consuming_unit_readings (uc_id, reading_at);
CREATE INDEX IF NOT EXISTS idx_transformer_balance_transformer_period
    ON transformer_balance (transformer_id, period_start DESC);
CREATE INDEX IF NOT EXISTS idx_consuming_units_tenant
    ON consuming_units (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_transformer_readings_import
    ON transformer_readings (import_id) WHERE import_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_consuming_unit_readings_import
    ON consuming_unit_readings (import_id) WHERE import_id IS NOT NULL;

-- ============================================================================
-- 8. Correcao do contador de assentos (migracao 007)
-- ============================================================================
-- A 007 ligou update_tenant_seat_count() (feita para users, le OLD.active)
-- tambem em invites, que nao tem a coluna active: aceitar um convite falhava
-- com "record old has no field active". O usuario criado no aceite ja
-- incrementa seat_count pelo gatilho de users, entao o gatilho de invites
-- e removido (ele tambem contaria o assento duas vezes).
DROP TRIGGER IF EXISTS trigger_update_tenant_seat_count_invite ON invites;
