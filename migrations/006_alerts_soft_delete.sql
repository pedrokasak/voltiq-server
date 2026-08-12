-- Migration 006: Add soft delete and updated_at to alerts table
-- Domain: Alerts

ALTER TABLE alerts ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

-- Update existing rows to have updated_at = created_at
UPDATE alerts SET updated_at = created_at WHERE updated_at IS NULL;

-- Update index to include deleted_at filter
DROP INDEX IF EXISTS idx_alerts_tenant;
CREATE INDEX idx_alerts_tenant ON alerts (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_alerts_transformer ON alerts (transformer_id) WHERE deleted_at IS NULL;