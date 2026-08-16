# CLAUDE.md — Voltiq

Leia este arquivo antes de qualquer tarefa. Define o contexto completo do projeto,
convenções e o que está pronto vs o que falta.

---

## O que é o Voltiq

SaaS B2B brasileiro para cálculo automático de balanço de energia e perdas técnicas
em transformadores de distribuição elétrica, seguindo o PRODIST Módulo 7 (ANEEL).

**Público-alvo:** Cooperativas de eletrificação rural e empresas de gestão de GD solar.
**Stack:** Go 1.22 (backend) + React/TypeScript (frontend) + PostgreSQL/Supabase + Fly.io.
**Repositórios:** voltiq-sw (server) e voltiq-energy-flow (web).

---

## Estado atual do projeto (Agosto 2026)

### ✅ Completo e funcionando
- Auth JWT + RLS + multitenancy completo
- Motor de cálculo PRODIST M7 (100% test coverage)
- CRUD transformadores + UCs (30+ endpoints)
- Importação CSV com validação linha a linha
- Dashboard com 8 visualizações conectadas à API real
- Export PDF + Excel
- Billing completo via Asaas (webhook, dunning, proration, métricas)
- TenantStatusMiddleware (bloqueia trial expirado/suspenso)
- Frontend 100% conectado à API real (zero mocks)
- Deploy backend no Fly.io: https://voltiq-sw-server.fly.dev
- Supabase como banco (PostgreSQL + TimescaleDB)

### ⚠️ Parcialmente implementado
- Alertas por email: usecase pronto, envio via Resend NÃO implementado
- Alert config por trafo: handler existe, falta ligar ao emailSvc

### ❌ Não implementado (MVP pendente)
- VITE_API_URL ainda aponta localhost:8080 (frontend não conectado em produção)
- Wizard onboarding multi-step (Sprint 3)
- Tela /settings/users — gestão de usuários do tenant
- Notificações in-app (sino + painel)
- Job agendado de cálculo automático diário
- Integração por email (CSV via email automatizado)

---

## Arquitetura

```
server/
  cmd/api/main.go                    ← entry point, wiring
  internal/
    calc/                            ← motor PRODIST M7 (NÃO ALTERAR sem testes)
    delivery/
      handler/                       ← handlers HTTP por domínio
      middleware/                    ← auth, rate limit, tenant status, permissions
      router/router.go               ← todas as rotas registradas
    domain/                          ← tipos compartilhados
    email/                           ← serviço Resend (service.go + templates.go)
    ingestion/                       ← parser CSV
    repository/                      ← acesso ao banco
    usecase/                         ← lógica de negócio
  migrations/                        ← SQL sequencial (001–010)

web/
  src/
    api/                             ← chamadas HTTP (auth, transformers, balance, etc)
    components/                      ← componentes React reutilizáveis
    hooks/                           ← useAuth, useExport, usePermissions
    pages/                           ← Dashboard, Transformers, Balance, Imports, Billing
    store/                           ← authStore (Zustand, token em memória)
    types/api.ts                     ← tipos TypeScript alinhados com backend
```

---

## Regras absolutas — nunca violar

### Backend (Go)
- NUNCA modificar migrations 001–010. Novas features = nova migration numerada.
- NUNCA fazer DROP TABLE ou DELETE sem confirmação explícita.
- NUNCA armazenar token ou senha em texto plano.
- NUNCA logar dados sensíveis (tokens, senhas, dados pessoais).
- NUNCA fazer retry em operações de escrita no banco.
- SEMPRE usar transações atômicas para escritas em múltiplas tabelas.
- SEMPRE verificar limites do plano antes de criar usuário ou transformador.
- SEMPRE aplicar RLS via `SET LOCAL app.tenant_id` em cada transação.
- Erros sempre retornados — nunca silenciados.
- Logs com slog estruturado, campos em inglês.

### Frontend (TypeScript/React)
- NUNCA usar `any` — TypeScript strict mode.
- NUNCA armazenar token em localStorage/sessionStorage — apenas Zustand em memória.
- NUNCA usar `dangerouslySetInnerHTML`.
- NUNCA fazer retry automático em 401/403/429.
- SEMPRE usar DemoBadge quando dados forem fallback/mock.
- SEMPRE skeleton loader durante queries — nunca spinner global bloqueante.
- SEMPRE toast de erro genérico para o usuário, sem expor detalhes internos.
- Mutations: retry: false sempre.

---

## Convenções de código

### Go
```go
// Comentários de regras de negócio em português
// CalculaBalancoTrafo calcula conforme PRODIST M7, Seção 6.2
func CalculaBalancoTrafo(e EntradaCalculo) (ResultadoBalanco, error) {
    // ...
}

// Respostas HTTP via helpers:
request.JSON(w, http.StatusOK, data)
request.Error(w, http.StatusBadRequest, "mensagem clara em português")

// Logs estruturados:
slog.Info("alert sent", "alert_id", id, "transformer", code, "loss_pct", loss)
```

### Banco de dados
```sql
-- Toda tabela nova precisa de:
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
tenant_id UUID NOT NULL REFERENCES tenants(id)
created_at TIMESTAMPTZ DEFAULT NOW()
deleted_at TIMESTAMPTZ NULL  -- soft delete

-- RLS obrigatório em toda tabela de negócio
ALTER TABLE nova_tabela ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON nova_tabela
    USING (tenant_id = current_setting('app.tenant_id')::UUID);
```

### TypeScript
```typescript
// Query keys sempre com tenantId para evitar colisão entre tenants
const keys = {
  list: (tenantId: string) => ['transformers', tenantId] as const,
  detail: (tenantId: string, id: string) => ['transformers', tenantId, id] as const,
}

// Tooltip e legendas dos gráficos sempre customizados (nunca default do Recharts)
const tooltipStyle = {
  contentStyle: {
    background: 'hsl(var(--popover))',
    border: '1px solid hsl(var(--border))',
    borderRadius: 8, fontSize: 12,
  },
}
```

---

## Papéis de usuário (RBAC)

```
SUPER_ADMIN    → Pedro — acesso total à plataforma, Painel Master
TENANT_ADMIN   → admin da cooperativa cliente — gerencia usuários e configurações
MANAGER        → gerente operacional — vê dashboard financeiro (R$)
ENGINEER       → engenheiro de campo — sem R$, sem edição de cadastros
VIEWER         → somente leitura
```

O dashboard financeiro (custo de perdas, receita, economia) é restrito a
SUPER_ADMIN, TENANT_ADMIN e MANAGER. ENGINEER e VIEWER veem apenas métricas
operacionais (%, kWh, status).

---

## Variáveis de ambiente obrigatórias

### Backend (.env / Fly.io secrets)
```env
DATABASE_URL=postgres://...supabase.co:5432/postgres?sslmode=require
JWT_SECRET=string-aleatoria-longa-minimo-32-chars
RESEND_API_KEY=re_xxx                    # alertas + emails billing
EMAIL_FROM=alertas@voltiq.com.br
EMAIL_FROM_NAME=Voltiq
ASAAS_API_KEY=aact_xxx                   # gateway de pagamento
ASAAS_WEBHOOK_KEY=xxx                    # validação de webhooks
ASAAS_SANDBOX=false                      # true em dev, false em prod
DASHBOARD_URL=https://app.voltiq.com.br
CORS_ALLOWED_ORIGINS=https://app.voltiq.com.br
PORT=8080
```

### Frontend (.env / Vercel)
```env
VITE_API_URL=https://voltiq-sw-server.fly.dev/api/v1
VITE_APP_ENV=production
```

---

## URLs de produção

- Backend: https://voltiq-sw-server.fly.dev
- Health: https://voltiq-sw-server.fly.dev/health
- Frontend: (deploy pendente no Vercel/Cloudflare)
- Banco: Supabase (dashboard em supabase.com)

---

## O que NUNCA fazer sem perguntar primeiro

1. Alterar o motor de cálculo em `internal/calc/` — qualquer mudança afeta conformidade PRODIST
2. Modificar migrations existentes (001–010)
3. Alterar a lógica de RLS ou multitenancy
4. Mudar o schema de autenticação (JWT payload, refresh token)
5. Fazer deploy em produção sem rodar testes (`go test ./...`)
6. Adicionar dependência nova sem justificativa no AGENTS.md

---

## Fluxo de trabalho com Claude Code

1. Leia este CLAUDE.md primeiro
2. Se a tarefa envolve banco → leia migrations existentes antes
3. Se a tarefa envolve cálculo PRODIST → leia `internal/calc/prodist_m7.go`
4. Se a tarefa envolve billing → leia `internal/usecase/billing_usecase.go`
5. Implemente seguindo as convenções acima
6. Gere testes junto com o código (especialmente para `internal/calc/`)
7. Nunca commite direto na `main` — use branch de feature

---

## Referências importantes

- PRODIST Módulo 7 (ANEEL): metodologia de cálculo de perdas
- SDD/: especificação técnica completa do produto
- SDD/08-ANEXOS/glossario.md: termos do domínio elétrico
- SDD/09-MODULO-GD/: spec do Módulo GD Solar (Fase 2)
- SDD/10-PRIVILEGIOS/: sistema de papéis e permissões
- SDD/11-BILLING-ONBOARDING/: ciclo de vida do cliente
