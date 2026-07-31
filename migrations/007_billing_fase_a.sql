-- Migration 007: Billing Fase A - Tenant Status and Limits
-- Domain: Billing
-- Execute AFTER 006_alerts_soft_delete.sql

-- 1. Add billing control columns to tenants table
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'TRIAL'
        CHECK (status IN ('TRIAL','ACTIVE','SUSPENDED','PENDING_PAYMENT','CANCELLED')),
    ADD COLUMN IF NOT EXISTS max_users INTEGER NOT NULL DEFAULT 5,
    ADD COLUMN IF NOT EXISTS seat_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS trial_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS plan TEXT NOT NULL DEFAULT 'trial'
        CHECK (plan IN ('trial','starter','pro','enterprise')),
    ADD COLUMN IF NOT EXISTS features JSONB DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS activated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

-- 2. Indexes for billing queries
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants (status);
CREATE INDEX IF NOT EXISTS idx_tenants_trial_expires ON tenants (trial_expires_at) WHERE trial_expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tenants_plan ON tenants (plan);

-- 3. Comments for documentation
COMMENT ON COLUMN tenants.status IS 'Estado do tenant para billing: TRIAL, ACTIVE, SUSPENDED, PENDING_PAYMENT, CANCELLED';
COMMENT ON COLUMN tenants.max_users IS 'Limite máximo de usuários (seats) permitidos no plano';
COMMENT ON COLUMN tenants.seat_count IS 'Contador atual de usuários ativos no tenant';
COMMENT ON COLUMN tenants.trial_expires_at IS 'Data de expiração do trial (se aplicável)';
COMMENT ON COLUMN tenants.plan IS 'Plano atual: trial, starter, pro, enterprise';
COMMENT ON COLUMN tenants.features IS 'JSONB gate para módulos: gd_module, scada_integration, ai_anomaly_detection';
COMMENT ON COLUMN tenants.activated_at IS 'Timestamp de ativação pós-pagamento';
COMMENT ON COLUMN tenants.suspended_at IS 'Timestamp de suspensão por inadimplência';
COMMENT ON COLUMN tenants.cancelled_at IS 'Timestamp de cancelamento (irreversível)';

-- 4. Default plan limits per tier
-- trial: max_users=5, starter: max_users=10, pro: max_users=50, enterprise: 999999 (unlimited)

-- 5. Update existing tenants to have proper defaults
UPDATE tenants
SET
    status = 'TRIAL',
    max_users = 5,
    seat_count = (
        SELECT COUNT(*)
        FROM users
        WHERE users.tenant_id = tenants.id
        AND users.deleted_at IS NULL
        AND users.active = true
    ),
    trial_expires_at = COALESCE(trial_expires_at, created_at + INTERVAL '14 days'),
    plan = 'trial',
    features = '{}'::jsonb
WHERE status IS NULL OR status = '';

-- 6. Trigger function for seat_count sync
CREATE OR REPLACE FUNCTION update_tenant_seat_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE tenants
        SET seat_count = seat_count + 1, updated_at = NOW()
        WHERE id = NEW.tenant_id;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE tenants
        SET seat_count = seat_count - 1, updated_at = NOW()
        WHERE id = OLD.tenant_id;
        RETURN OLD;
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.tenant_id != NEW.tenant_id THEN
            UPDATE tenants
            SET seat_count = seat_count - 1, updated_at = NOW()
            WHERE id = OLD.tenant_id;
            UPDATE tenants
            SET seat_count = seat_count + 1, updated_at = NOW()
            WHERE id = NEW.tenant_id;
        ELSIF OLD.active != NEW.active THEN
            IF NEW.active THEN
                UPDATE tenants
                SET seat_count = seat_count + 1, updated_at = NOW()
                WHERE id = NEW.tenant_id;
            ELSE
                UPDATE tenants
                SET seat_count = seat_count - 1, updated_at = NOW()
                WHERE id = NEW.tenant_id;
            END IF;
        END IF;
        RETURN NEW;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Trigger on users table
DROP TRIGGER IF EXISTS trigger_update_tenant_seat_count ON users;
CREATE TRIGGER trigger_update_tenant_seat_count
AFTER INSERT OR DELETE OR UPDATE OF tenant_id, active ON users
FOR EACH ROW EXECUTE FUNCTION update_tenant_seat_count();

-- Trigger on invites (when accepted)
DROP TRIGGER IF EXISTS trigger_update_tenant_seat_count_invite ON invites;
CREATE TRIGGER trigger_update_tenant_seat_count_invite
AFTER UPDATE OF status ON invites
FOR EACH ROW
WHEN (OLD.status = 'PENDING' AND NEW.status = 'ACCEPTED')
EXECUTE FUNCTION update_tenant_seat_count();