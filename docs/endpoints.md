# EnergyBalance API Documentation

## Visão Geral

API REST para cálculo de balanço de energia e perdas técnicas em transformadores de distribuição, seguindo a regulamentação ANEEL (PRODIST Módulo 7).

**Base URL:** `http://localhost:8080/api/v1`

**Autenticação:** Bearer Token (JWT)

```bash
Authorization: Bearer <your_token_here>
```

---

## Índice

1. [Health & Metrics](#health--metrics)
2. [Autenticação](#autenticação)
3. [Convites (Invites)](#convites-invites)
4. [Transformadores](#transformadores)
5. [Unidades Consumidoras](#unidades-consumidoras)
6. [Balanço de Energia](#balanço-de-energia)
7. [Importação CSV](#importação-csv)

---

## Health & Metrics

### GET /health
Health check para liveness probe.

**Auth:** Não requer

**Response:**
```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "timestamp": "2025-01-15T10:30:00Z",
    "version": "0.1.0"
  }
}
```

### GET /ready
Readiness probe para verificar se a API está pronta para receber tráfego.

**Auth:** Não requer

**Response:**
```json
{
  "success": true,
  "data": {
    "status": "ready",
    "timestamp": "2025-01-15T10:30:00Z",
    "version": "0.1.0",
    "checks": {
      "database": "ok",
      "migrations": "ok"
    }
  }
}
```

### GET /metrics
Métricas Prometheus para monitoramento.

**Auth:** Não requer

**Response (text/plain):**
```
# HELP api_requests_total Total number of API requests
# TYPE api_requests_total counter
api_requests_total{method="GET",endpoint="/transformers"} 1542
api_requests_total{method="POST",endpoint="/imports/transformers"} 89
# HELP api_uptime_seconds API uptime in seconds
# TYPE api_uptime_seconds gauge
api_uptime_seconds 3600
```

---

## Autenticação

### POST /auth/signup
Cria uma nova conta (tenant) e usuário admin. Self-service onboarding.

**Auth:** Não requer

**Request:**
```json
{
  "tenant_name": "Cooperativa de Energia Rural",
  "tenant_document": "12.345.678/0001-90",
  "plan": "trial",
  "admin_name": "João Silva",
  "admin_email": "joao@cooperativa.com.br",
  "admin_password": "SenhaForte123!"
}
```

**Planos disponíveis:** `trial`, `starter`, `pro`, `enterprise`

**Response (201):**
```json
{
  "success": true,
  "data": {
    "tenant": {
      "id": "uuid",
      "name": "Cooperativa de Energia Rural",
      "document": "12.345.678/0001-90",
      "plan": "trial",
      "trial_until": "2025-02-14T00:00:00Z",
      "active": true
    },
    "user": {
      "id": "uuid",
      "tenant_id": "uuid",
      "email": "joao@cooperativa.com.br",
      "name": "João Silva",
      "role": "ADMIN"
    }
  },
  "message": "account created successfully"
}
```

### POST /auth/login
Autentica usuário e retorna token JWT.

**Auth:** Não requer

**Request:**
```json
{
  "email": "joao@cooperativa.com.br",
  "password": "SenhaForte123!"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_at": "2025-01-16T10:30:00Z",
    "user": {
      "id": "uuid",
      "tenant_id": "uuid",
      "email": "joao@cooperativa.com.br",
      "name": "João Silva",
      "role": "ADMIN"
    }
  },
  "message": "login successful"
}
```

### POST /auth/refresh
Renova o token de acesso usando refresh token.

**Auth:** Não requer (usa refresh token)

**Request:**
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "token": "novo_access_token",
    "expires_at": "2025-01-16T10:30:00Z",
    "refresh_token": "novo_refresh_token",
    "refresh_expiry": "2025-01-22T10:30:00Z"
  },
  "message": "token refreshed successfully"
}
```

### POST /auth/logout
Invalida tokens (logout).

**Auth:** Requer Bearer token

**Request:**
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Response (200):**
```json
{
  "success": true,
  "message": "logout successful"
}
```

---

## Convites (Invites)

### POST /invites
Cria um convite para novo usuário (requer role ADMIN).

**Auth:** Requer Bearer token (ADMIN)

**Request:**
```json
{
  "email": "maria@cooperativa.com.br",
  "role": "ENGINEER"
}
```

**Roles disponíveis:** `ADMIN`, `ENGINEER`, `VIEWER`

**Response (201):**
```json
{
  "success": true,
  "data": {
    "invite_id": "uuid",
    "email": "maria@cooperativa.com.br",
    "role": "ENGINEER",
    "token": "token-unico-para-aceite",
    "expires_at": "2025-01-22T10:30:00Z",
    "status": "PENDING"
  },
  "message": "invite created successfully"
}
```

### GET /invites/{token}
Valida um token de convite.

**Auth:** Não requer

**Response (200):**
```json
{
  "success": true,
  "data": {
    "invite_id": "uuid",
    "email": "maria@cooperativa.com.br",
    "role": "ENGINEER",
    "tenant_id": "uuid",
    "expires_at": "2025-01-22T10:30:00Z",
    "status": "PENDING"
  },
  "message": "invite is valid"
}
```

### POST /invites/{token}/accept
Aceita um convite e cria a conta do usuário.

**Auth:** Não requer

**Request:**
```json
{
  "name": "Maria Santos",
  "password": "SenhaForte123!"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid",
      "tenant_id": "uuid",
      "email": "maria@cooperativa.com.br",
      "name": "Maria Santos",
      "role": "ENGINEER",
      "active": true
    }
  },
  "message": "invite accepted successfully"
}
```

### DELETE /invites/{id}
Cancela um convite pendente.

**Auth:** Requer Bearer token (ADMIN)

**Response (200):**
```json
{
  "success": true,
  "message": "invite cancelled successfully"
}
```

---

## Transformadores

### POST /api/v1/transformers
Cria um novo transformador.

**Auth:** Requer Bearer token

**Request:**
```json
{
  "code": "TRF-001",
  "power_kva": 112.5,
  "primary_voltage_kv": 13.8,
  "secondary_voltage_v": 220,
  "lat": -23.550520,
  "lng": -46.633308,
  "core_loss_kw": 0.340,
  "winding_loss_kw": 1.450,
  "loss_limit_pct": 10.0,
  "substation_id": "uuid-opcional"
}
```

**Response (201):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "tenant_id": "uuid",
    "code": "TRF-001",
    "power_kva": 112.5,
    "primary_voltage_kv": 13.8,
    "secondary_voltage_v": 220,
    "lat": -23.550520,
    "lng": -46.633308,
    "core_loss_kw": 0.340,
    "winding_loss_kw": 1.450,
    "loss_limit_pct": 10.0,
    "active": true,
    "created_at": "2025-01-15T10:30:00Z",
    "updated_at": "2025-01-15T10:30:00Z"
  },
  "message": "transformer created successfully"
}
```

### GET /api/v1/transformers
Lista todos os transformadores do tenant.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "tenant_id": "uuid",
      "code": "TRF-001",
      "power_kva": 112.5,
      "active": true
    }
  ]
}
```

### GET /api/v1/transformers/{id}
Obtém detalhes de um transformador.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "code": "TRF-001",
    "power_kva": 112.5,
    "primary_voltage_kv": 13.8,
    "secondary_voltage_v": 220,
    "lat": -23.550520,
    "lng": -46.633308,
    "core_loss_kw": 0.340,
    "winding_loss_kw": 1.450,
    "loss_limit_pct": 10.0,
    "active": true
  }
}
```

### PUT /api/v1/transformers/{id}
Atualiza um transformador.

**Auth:** Requer Bearer token

**Request:** (mesmo schema de criação, campos opcionais)

**Response (200):**
```json
{
  "success": true,
  "data": { ... },
  "message": "transformer updated successfully"
}
```

### DELETE /api/v1/transformers/{id}
Remove um transformador (soft delete - atualiza `deleted_at`).

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "message": "transformer deleted successfully"
}
```

**Nota:** Este endpoint realiza **soft delete** atualizando o campo `deleted_at`, não remove o registro do banco. Conforme SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md.

### GET /api/v1/transformers/count
Conta total de transformadores.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "count": 42
  }
}
```

### GET /api/v1/transformers/{id}/technical-data
Obtém dados técnicos do transformador.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "transformer_id": "uuid",
    "code": "TRF-001",
    "power_kva": 112.5,
    "primary_voltage_kv": 13.8,
    "secondary_voltage_v": 220,
    "core_loss_kw": 0.340,
    "winding_loss_kw": 1.450,
    "nominal_current": 295.08
  }
}
```

### GET /api/v1/transformers/{id}/location
Obtém localização do transformador.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "transformer_id": "uuid",
    "code": "TRF-001",
    "lat": -23.550520,
    "lng": -46.633308
  }
}
```

---

## Unidades Consumidoras

### POST /api/v1/consuming-units
Cria uma nova unidade consumidora.

**Auth:** Requer Bearer token

**Request:**
```json
{
  "transformer_id": "uuid",
  "uc_code": "UC-001",
  "name": "Residência João Silva",
  "class": "RESIDENTIAL",
  "active": true
}
```

**Classes disponíveis:** `RESIDENTIAL`, `COMMERCIAL`, `INDUSTRIAL`, `RURAL`, `PUBLIC_POWER`

**Response (201):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "tenant_id": "uuid",
    "transformer_id": "uuid",
    "uc_code": "UC-001",
    "name": "Residência João Silva",
    "class": "RESIDENTIAL",
    "active": true
  },
  "message": "consuming unit created successfully"
}
```

### GET /api/v1/consuming-units
Lista todas as UCs do tenant.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": [...]
}
```

### GET /api/v1/consuming-units/transformer/{transformer_id}
Lista UCs vinculadas a um transformador.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": [...]
}
```

### PUT /api/v1/consuming-units/{id}
Atualiza uma UC.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": { ... },
  "message": "consuming unit updated successfully"
}
```

### DELETE /api/v1/consuming-units/{id}
Remove uma UC (soft delete).

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "message": "consuming unit deleted successfully"
}
```

---

## Balanço de Energia

### POST /api/v1/balance/transformer/{transformer_id}/calculate
Calcula o balanço de energia para um transformador.

**Auth:** Requer Bearer token

**Request:**
```json
{
  "period_start": "2025-01-01",
  "period_end": "2025-01-31"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "tenant_id": "uuid",
    "transformer_id": "uuid",
    "period_start": "2025-01-01",
    "period_end": "2025-01-31",
    "energy_injected_kwh": 15000.50,
    "total_consumption_kwh": 13500.25,
    "loss_kwh": 1500.25,
    "loss_pct": 10.00,
    "status": "NORMAL",
    "uc_count": 45,
    "calculated_at": "2025-01-15T10:30:00Z"
  },
  "message": "balance calculated successfully"
}
```

**Status possíveis:** `NORMAL`, `WARNING`, `CRITICAL`

### GET /api/v1/balance/transformer/{transformer_id}
Lista balanços por período.

**Auth:** Requer Bearer token

**Query Params:**
- `period_start` (YYYY-MM-DD)
- `period_end` (YYYY-MM-DD)

**Response (200):**
```json
{
  "success": true,
  "data": [...]
}
```

### GET /api/v1/balance/transformer/{transformer_id}/latest
Obtém o último balanço calculado.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": { ... }
}
```

### GET /api/v1/balance/transformer/{transformer_id}/technical-loss
Calcula perdas técnicas conforme PRODIST Módulo 7.

**Auth:** Requer Bearer token

**Query Params:**
- `period_start` (YYYY-MM-DD)
- `period_end` (YYYY-MM-DD)

**Response (200):**
```json
{
  "success": true,
  "data": {
    "transformer_id": "uuid",
    "tenant_id": "uuid",
    "energy_injected_kwh": 15000.50,
    "total_consumption_kwh": 13500.25,
    "loss_kwh": 1500.25,
    "loss_pct": 10.00,
    "technical_loss_trafo_kwh": 1200.00,
    "non_technical_loss_kwh": 300.25,
    "status": "NORMAL",
    "limit_pct": 10.0,
    "uc_count": 45,
    "calculated_at": "2025-01-15T10:30:00Z"
  }
}
```

---

## Importação CSV

### POST /api/v1/imports/transformers
Faz upload de CSV com leituras de transformadores.

**Auth:** Requer Bearer token

**Content-Type:** `multipart/form-data`

**Form Data:**
- `file`: arquivo CSV

**Formato do CSV:**
```csv
transformer_id,reading_at,energy_kwh,demand_kw,power_factor
uuid,2025-01-15 10:00:00,1500.50,120.5,0.92
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "import_id": "uuid",
    "status": "COMPLETED",
    "total_rows": 1000,
    "rows_ok": 950,
    "rows_error": 50,
    "message": "imported 950 readings successfully"
  }
}
```

### POST /api/v1/imports/ucs
Faz upload de CSV com leituras de UCs.

**Auth:** Requer Bearer token

**Formato do CSV:**
```csv
uc_id,transformer_id,reading_at,consumption_kwh
uuid,uuid,2025-01-15 10:00:00,250.50
```

### GET /api/v1/imports
Lista todas as importações do tenant.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "imports": [...],
    "count": 15
  }
}
```

### GET /api/v1/imports/{id}
Obtém detalhes de uma importação.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "import_id": "uuid",
    "file_name": "leituras_jan_2025.csv",
    "status": "COMPLETED",
    "total_rows": 1000,
    "rows_ok": 950,
    "rows_error": 50,
    "errors": {
      "line_45": "UC not found",
      "line_78": "invalid date format"
    },
    "created_at": "2025-01-15T10:30:00Z",
    "completed_at": "2025-01-15T10:35:00Z"
  }
}
```

### GET /api/v1/imports/{id}/status
Obtém status detalhado com progresso.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "import_id": "uuid",
    "file_name": "leituras_jan_2025.csv",
    "status": "PROCESSING",
    "progress_pct": 75,
    "total_rows": 1000,
    "rows_ok": 750,
    "rows_error": 0,
    "errors": null,
    "created_at": "2025-01-15T10:30:00Z",
    "completed_at": null
  }
}
```

### GET /api/v1/imports/summary
Obtém resumo estatístico das importações.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "total_imports": 15,
    "completed": 12,
    "processing": 2,
    "failed": 1,
    "total_rows": 15000,
    "total_rows_ok": 14250,
    "total_rows_error": 750,
    "success_rate": 95.0
  }
}
```

### GET /api/v1/imports/history
Histórico de importações com filtros.

**Auth:** Requer Bearer token

**Query Params:**
- `status`: `PROCESSING`, `COMPLETED`, `ERROR`
- `limit`: número máximo de registros (default: 50)

### GET /api/v1/imports/{id}/errors
Baixa relatório de erros de uma importação.

**Auth:** Requer Bearer token

**Response (200):**
```json
{
  "success": true,
  "data": {
    "line_45": "UC not found",
    "line_78": "invalid date format"
  }
}
```

---

## Códigos de Erro

| Código | HTTP Status | Descrição |
|--------|-------------|-----------|
| `VALIDATION_ERROR` | 400 | Erro de validação de campos |
| `UNAUTHORIZED` | 401 | Token inválido ou expirado |
| `FORBIDDEN` | 403 | Permissões insuficientes |
| `NOT_FOUND` | 404 | Recurso não encontrado |
| `CREATE_ERROR` | 500 | Erro ao criar recurso |
| `UPDATE_ERROR` | 500 | Erro ao atualizar recurso |
| `DELETE_ERROR` | 500 | Erro ao deletar recurso |
| `LIST_ERROR` | 500 | Erro ao listar recursos |
| `CALCULATION_ERROR` | 500 | Erro no cálculo de balanço |
| `IMPORT_ERROR` | 500 | Erro na importação CSV |
| `INVALID_CREDENTIALS` | 401 | Email ou senha inválidos |
| `SIGNUP_ERROR` | 400 | Erro no cadastro |
| `INVITE_ERROR` | 400 | Erro no convite |
| `INVITE_INVALID` | 400 | Convite inválido |
| `INVITE_ACCEPT_ERROR` | 400 | Erro ao aceitar convite |
| `INVITE_CANCEL_ERROR` | 400 | Erro ao cancelar convite |

---

## Exemplos de Uso com cURL

### Login
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"senha123"}'
```

### Criar Transformador
```bash
curl -X POST http://localhost:8080/api/v1/transformers \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "code": "TRF-001",
    "power_kva": 112.5,
    "primary_voltage_kv": 13.8,
    "secondary_voltage_v": 220
  }'
```

### Upload CSV
```bash
curl -X POST http://localhost:8080/api/v1/imports/transformers \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@leituras.csv"
```

### Health Check
```bash
curl http://localhost:8080/health
```

---

## Considerações Finais

- **Soft Delete:** Todos os endpoints DELETE realizam soft delete (atualizam `deleted_at`), conforme SDD.
- **Multitenancy:** Todos os recursos são isolados por `tenant_id`.
- **Rate Limiting:** Implementar conforme necessidade (env var `RATE_LIMIT_REQUESTS_PER_MINUTE`).
- **CORS:** Configurável via `CORS_ALLOWED_ORIGINS`.
