# Voltiq Server

API em Go do Voltiq: balanço de energia e perdas técnicas em transformadores
de distribuição conforme ANEEL PRODIST Módulo 7, multitenant, com cobrança via
Asaas, alertas por e-mail (Resend) e WhatsApp (Z-API).

## Estrutura

```
cmd/api/                 entrada (servidor HTTP e comando "cron")
internal/
  app/                   montagem das dependências (usada pelo main e pelos testes)
  calc/                  motor PRODIST M7 (não alterar sem testes)
  delivery/handler/      handlers HTTP
  delivery/middleware/   auth, RBAC, rate limit, status do tenant, segurança
  delivery/router/       rotas e permissões
  domain/                tipos, papéis/permissões, limites de plano, validações
  email/ notify/         Resend e Z-API
  aneel/                 dados abertos da ANEEL (tarifas homologadas)
  ingestion/             leitura de CSV/XLSX, mapeamento de colunas, validação
  jobs/                  agendador em processo (trial, cobrança, balanço diário...)
  repository/            acesso ao banco (filtro por tenant + variáveis de RLS)
  usecase/               regras de negócio
migrations/              SQL numerado (001-013)
scripts/migrate.sh       aplica as migrações em ordem
tests/                   integração (exige banco) e segurança
docs/endpoints.md        catálogo de rotas, permissões e jobs
```

## Rodando localmente

```bash
cp .env.example .env              # ajuste DATABASE_URL e JWT_SECRET
DATABASE_URL=... scripts/migrate.sh
go run ./cmd/api                  # API em :8080
go run ./cmd/api cron             # executa todos os jobs uma vez
```

Requer PostgreSQL 16 com TimescaleDB (ou Supabase). As variáveis estão
documentadas em `.env.example`.

## Testes

```bash
go test ./...                                  # unidade (integração é pulada sem banco)
DATABASE_URL=... go test ./... -count=1        # inclui integração ponta a ponta
go test -cover ./internal/calc/                # motor de cálculo (>= 80%)
```

O CI (`.github/workflows/ci.yml`) roda gofmt, vet, migrações em TimescaleDB,
todos os testes e a checagem de cobertura do motor.

## Multitenancy

Cada requisição autenticada carrega a identidade (`internal/reqctx`). Os
repositórios filtram por `tenant_id` e o pool aplica `app.tenant_id` /
`app.current_tenant_id` em cada conexão para as políticas de RLS, que a
migração 011 unificou em `app_current_tenant()`. Recursos de outro tenant
respondem 404.

## Deploy

Fly.io (`fly.toml`), região `gru`, uma máquina sempre ligada porque os jobs
agendados rodam no processo da API. Rodar `go test ./...` antes do deploy.
