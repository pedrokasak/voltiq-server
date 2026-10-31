-- Reversao da migracao 011. Remove os objetos criados por ela e restaura as
-- politicas de tenant no formato das migracoes 002-006. Apaga os dados de
-- destinatarios e grupos de alerta: rodar apenas com backup.

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'transformers', 'consuming_units', 'substations', 'imports',
        'users', 'invites', 'alert_recipients', 'transformer_groups', 'tenants'
    ] LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS audit_%s ON %I', t, t);
    END LOOP;
END $$;
DROP FUNCTION IF EXISTS audit_row_change();

-- Restaura o gatilho da 007 (com o defeito original em invites).
DROP TRIGGER IF EXISTS trigger_update_tenant_seat_count_invite ON invites;
CREATE TRIGGER trigger_update_tenant_seat_count_invite
AFTER UPDATE OF status ON invites
FOR EACH ROW
WHEN (OLD.status = 'PENDING' AND NEW.status = 'ACCEPTED')
EXECUTE FUNCTION update_tenant_seat_count();

DROP INDEX IF EXISTS idx_transformer_readings_transformer_time;
DROP INDEX IF EXISTS idx_consuming_unit_readings_uc_time;
DROP INDEX IF EXISTS idx_transformer_balance_transformer_period;
DROP INDEX IF EXISTS idx_consuming_units_tenant;
DROP INDEX IF EXISTS idx_transformer_readings_import;
DROP INDEX IF EXISTS idx_consuming_unit_readings_import;
DROP INDEX IF EXISTS idx_imports_tenant_created;
DROP INDEX IF EXISTS idx_alerts_tenant_created;

ALTER TABLE tenants DROP COLUMN IF EXISTS tariff_brl_mwh;
ALTER TABLE tenants DROP COLUMN IF EXISTS onboarding_completed_at;

ALTER TABLE imports DROP COLUMN IF EXISTS reading_type;
ALTER TABLE imports DROP COLUMN IF EXISTS file_content;
ALTER TABLE imports DROP COLUMN IF EXISTS column_mapping;
ALTER TABLE imports DROP COLUMN IF EXISTS duplicates_skipped;
ALTER TABLE imports DROP COLUMN IF EXISTS updated_at;
ALTER TABLE imports DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE alerts DROP COLUMN IF EXISTS recipient_id;
ALTER TABLE alerts DROP COLUMN IF EXISTS error_message;
ALTER TABLE alerts DROP COLUMN IF EXISTS attempts;
ALTER TABLE alerts DROP COLUMN IF EXISTS loss_pct;
ALTER TABLE alerts DROP COLUMN IF EXISTS limit_pct;

DROP TABLE IF EXISTS alert_recipients;
DROP TABLE IF EXISTS transformer_group_members;
DROP TABLE IF EXISTS transformer_groups;

DROP POLICY IF EXISTS tenant_isolation_audit_log ON audit_log;
DROP POLICY IF EXISTS tenant_isolation_users ON users;
CREATE POLICY tenant_isolation_users ON users
    USING (tenant_id = current_setting('app.tenant_id')::UUID);
DROP POLICY IF EXISTS tenant_isolation_tenants ON tenants;
CREATE POLICY tenant_isolation_tenants ON tenants
    USING (id = current_setting('app.tenant_id')::UUID);

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'substations', 'transformers', 'consuming_units',
        'transformer_readings', 'consuming_unit_readings',
        'transformer_balance', 'imports', 'alerts'
    ] LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_%s ON %I', t, t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation_%s ON %I USING (tenant_id = current_setting(''app.tenant_id'')::UUID)',
            t, t);
    END LOOP;
END $$;
DROP POLICY IF EXISTS tenant_isolation_invites ON invites;
CREATE POLICY invite_tenant_isolation_policy ON invites
    USING (tenant_id = current_setting('app.current_tenant_id', true)::UUID);

DROP FUNCTION IF EXISTS app_current_tenant();
