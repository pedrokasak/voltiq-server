# Skill: go-migration
# Quando usar: adicionar qualquer nova tabela, coluna ou índice ao banco

## Regra absoluta
NUNCA modificar migrations 001–010 existentes.
Sempre criar arquivo novo com próximo número sequencial.

## Verificar qual é o próximo número
```bash
ls migrations/ | sort | tail -1
# Se última for 010_xxx.sql → próxima é 011_xxx.sql
```

## Template de migration completa

```sql
-- migrations/011_nome_descritivo.sql
-- Propósito: [descrever o que esta migration faz e por quê]
-- Sprint: Sprint X
-- Data: YYYY-MM-DD

-- ============================================================
-- NOVA TABELA
-- ============================================================
CREATE TABLE IF NOT EXISTS nome_tabela (
    -- Campos obrigatórios em toda tabela de negócio:
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ,  -- soft delete

    -- Campos específicos desta tabela:
    nome        TEXT NOT NULL,
    valor       DECIMAL(12,4),
    status      TEXT NOT NULL DEFAULT 'ATIVO'
                    CHECK (status IN ('ATIVO', 'INATIVO'))
);

-- ============================================================
-- ROW-LEVEL SECURITY (obrigatório em toda tabela de negócio)
-- ============================================================
ALTER TABLE nome_tabela ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON nome_tabela
    USING (tenant_id = current_setting('app.tenant_id')::UUID);

-- ============================================================
-- ÍNDICES (para queries frequentes)
-- ============================================================
CREATE INDEX idx_nome_tabela_tenant
    ON nome_tabela (tenant_id)
    WHERE deleted_at IS NULL;

-- Índice para busca por campo específico:
CREATE INDEX idx_nome_tabela_status
    ON nome_tabela (tenant_id, status)
    WHERE deleted_at IS NULL;

-- ============================================================
-- SÉRIE TEMPORAL (se a tabela tiver timestamp de evento)
-- Descomentar se aplicável:
-- ============================================================
-- SELECT create_hypertable('nome_tabela', 'evento_em', if_not_exists => true);
-- SELECT add_compression_policy('nome_tabela', INTERVAL '3 months');
```

## Adicionar coluna em tabela existente

```sql
-- migrations/011_add_campo_em_tabela.sql
ALTER TABLE tabela_existente
    ADD COLUMN IF NOT EXISTS novo_campo TEXT;

-- Se tem constraint:
ALTER TABLE tabela_existente
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'ATIVO'
        CHECK (status IN ('ATIVO', 'INATIVO'));

-- Migrar dados existentes se necessário:
UPDATE tabela_existente
    SET novo_campo = 'valor_padrao'
    WHERE novo_campo IS NULL;
```

## Alterar constraint de CHECK (ex: adicionar novo papel)

```sql
-- migrations/011_update_role_check.sql
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('SUPER_ADMIN', 'TENANT_ADMIN', 'MANAGER', 'ENGINEER', 'VIEWER'));

-- Migrar dados se necessário:
UPDATE users SET role = 'TENANT_ADMIN' WHERE role = 'ADMIN';
```

## Checklist antes de aplicar

- [ ] Arquivo numerado sequencialmente
- [ ] Comentário explicando propósito
- [ ] `IF NOT EXISTS` em CREATE TABLE e CREATE INDEX
- [ ] RLS habilitado em tabelas de negócio
- [ ] Índices para queries frequentes
- [ ] Migração de dados existentes se necessário
- [ ] Testado no SQL Editor do Supabase antes de commitar
- [ ] Sem DROP TABLE ou DROP COLUMN (soft delete preferido)
