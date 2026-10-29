-- Migration 013: progresso do treinamento in-app por usuário
-- Cada linha registra uma lição concluída (com a nota do quiz, quando houver).

CREATE TABLE IF NOT EXISTS training_progress (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    user_id      UUID NOT NULL REFERENCES users(id),
    lesson_key   TEXT NOT NULL CHECK (lesson_key ~ '^[a-z0-9_-]{1,64}$'),
    quiz_score   INTEGER CHECK (quiz_score BETWEEN 0 AND 100),
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_training_progress_user_lesson
    ON training_progress (user_id, lesson_key) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_training_progress_tenant
    ON training_progress (tenant_id) WHERE deleted_at IS NULL;

ALTER TABLE training_progress ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_training_progress ON training_progress;
CREATE POLICY tenant_isolation_training_progress ON training_progress
    USING (app_current_tenant() IS NULL OR tenant_id = app_current_tenant())
    WITH CHECK (app_current_tenant() IS NULL OR tenant_id = app_current_tenant());
