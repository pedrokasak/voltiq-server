# Voltiq API - catálogo de endpoints

Base: `https://voltiq-sw-server.fly.dev/api/v1` (local: `http://localhost:8080/api/v1`).
Autenticação: `Authorization: Bearer <token>`; o refresh token vai no cookie
HttpOnly `refresh_token`. Respostas usam o envelope:

```json
{ "success": true, "data": { }, "message": "" }
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "dados inválidos", "details": [{"field": "power_kva", "message": "..."}] } }
```

Códigos usuais: 401 sem sessão, 402 `PLAN_LIMIT_REACHED`/`TRIAL_EXPIRED`/`TENANT_SUSPENDED`,
403 `FORBIDDEN` (permissão), 404 (inclusive recurso de outro tenant), 409 conflito,
422 validação, 429 limite de requisições (`Retry-After`).

Datas sem fuso (`2026-09-01`) são interpretadas no horário de Brasília.

## Permissões (RBAC)

| Papel | Permissões |
|---|---|
| SUPER_ADMIN | todas + painel master `/admin` |
| OWNER, ADMIN | todas do tenant (usuários, plano, auditoria, R$) |
| MANAGER | operação + cadastros + dados financeiros (R$) |
| ENGINEER | visualização + importação, leituras, balanço, alertas; sem R$ e sem cadastros |
| VIEWER | somente visualização e exportação |

`GET /roles` devolve os papéis com a lista de permissões; `GET /auth/me` devolve
as permissões do usuário, limites do plano, uso e progresso do onboarding.

## Limites de requisição

| Categoria | Limite |
|---|---|
| login, signup, aceite de convite | 5/min por IP+UA |
| refresh | 10/min |
| importações | 20/min |
| cálculo de balanço | 30/min |
| demais | 60/min |

## Rotas

### Públicas
| Método | Rota | Descrição |
|---|---|---|
| POST | /auth/login | `{email, password}` -> `{token, user, expires_at}` + cookie |
| POST | /auth/signup | `{company_name, cnpj, name, email, password}`; cria tenant em trial (14 dias) e já autentica |
| POST | /auth/refresh | usa o cookie; devolve novo token e `user` |
| POST | /auth/logout | apaga o cookie |
| GET | /invites/{token} | valida convite |
| POST | /invites/{token}/accept | `{name, password}`; cria o usuário e autentica |
| POST | /webhooks/asaas | webhook do Asaas (assinatura HMAC) |

### Sessão, conta e usuários
| Método | Rota | Permissão |
|---|---|---|
| GET | /auth/me | autenticado |
| POST | /auth/change-password | autenticado |
| GET | /roles, /onboarding, /tenant/settings | autenticado |
| POST | /onboarding/complete | tenant:manage |
| PUT | /tenant/settings | tenant:manage (`tariff_brl_mwh`, endereço) |
| GET | /audit-logs?table=&operation=&user=&since=&until= | audit:view |
| GET | /users | users:manage (usuários + convites pendentes) |
| PATCH | /users/{id} | users:manage (`role`, `name`, `active`) |
| DELETE | /users/{id} | users:manage |
| POST/GET | /invites | users:manage (`{email, role}`; respeita assentos do plano) |
| DELETE | /invites/{id} | users:manage |
| POST | /invites/{id}/resend | users:manage |

### Rede elétrica
| Método | Rota | Permissão |
|---|---|---|
| GET | /substations, /substations/{id} | transformer:view |
| POST/PUT/DELETE | /substations, /substations/{id} | transformer:manage |
| GET | /transformers?search=&status=&substation_id=&group_id=&order_by=&page=&limit= | transformer:view |
| GET | /transformers/{id}, /{id}/stats, /{id}/loss-analysis, /{id}/readings/series, /{id}/top-consumers | transformer:view |
| POST/PUT/PATCH/DELETE | /transformers, /transformers/{id} (+ /batch) | transformer:manage (respeita limite do plano) |
| POST | /transformers/import | transformer:manage (cadastro em massa CSV/XLSX, `dry_run`) |
| POST | /transformers/{id}/readings | reading:manage (leitura manual, recalcula o mês) |
| GET | /consuming-units, /consuming-units/{id}, /{id}/consumption | uc:view |
| POST/PUT/PATCH/DELETE | /consuming-units, /consuming-units/{id} | uc:manage |
| POST | /consuming-units/import | uc:manage |
| POST | /consuming-units/{id}/readings | reading:manage |

### Importação de leituras (CSV ou XLSX)
| Método | Rota | Descrição |
|---|---|---|
| GET | /imports/schemas | campos aceitos e sinônimos de cabeçalho |
| GET | /imports/template?type=transformers\|ucs\|transformer_registry\|uc_registry | modelo CSV |
| POST | /imports/validate | multipart `file`, `type`, `mapping` (JSON); pré-visualização sem gravar |
| POST | /imports/transformers, /imports/ucs | importa; ignora duplicatas; recalcula balanços afetados |
| POST | /imports/batch | vários arquivos |
| GET | /imports, /imports/summary, /imports/{id}, /{id}/errors (CSV), /{id}/logs | histórico |
| POST | /imports/{id}/retry | reprocessa o arquivo guardado |
| DELETE | /imports/{id}?confirm=true | desfaz a importação (remove as leituras) |

### Balanço, alertas e risco
| Método | Rota | Permissão |
|---|---|---|
| POST | /balance/transformer/{id}/calculate | balance:calculate |
| POST | /balance/calculate-all | balance:calculate |
| GET | /balance/transformer/{id}, /latest, /technical-loss | reading:view |
| GET/POST/PUT/DELETE | /alert-recipients (+ /{id}/test) | alert:manage para escrita |
| GET/POST/PUT/DELETE | /transformer-groups | alert:manage para escrita |
| GET | /alerts?status=&channel=&type=&days=, /alerts/stats, /alerts/{id} | dashboard:view |
| GET | /risk/all-scores, /risk/all-anomalies, /risk/transformer/{id}/score | dashboard:view |

### Dashboards e mapa
| Método | Rota | Descrição |
|---|---|---|
| GET | /dashboard/map?status=&substation_id=&group_id=&search= | trafos georreferenciados com último balanço, fator de carga, leituras atrasadas, alertas, subestações, bounds e centro; R$ só para quem tem financial:view |
| GET | /dashboard/operational | rede, status, último mês x anterior, qualidade de dados, importações, alertas, trafos que pedem atenção, evolução 12 meses, subestações |
| GET | /dashboard/managerial | **financial:view** - tarifa do tenant, mês atual x anterior, acumulado do ano, 12 meses, projeção anual, por subestação, top valor recuperável |
| GET | /dashboard/kpis, /kpi-sparklines, /loss-evolution, /loss-composition, /status-distribution, /quarterly-injected-billed, /top-transformers, /transformer-current-status, /monthly-loss | gráficos |
| GET | /dashboard/financial-table, /dashboard/accumulated-loss-cost | **financial:view** |

### Relatórios
| Método | Rota | Permissão |
|---|---|---|
| GET | /exports/balance/{transformer_id}?format=pdf\|excel\|csv&start=&end= | report:export |
| GET | /exports/aneel?start=&end= | report:export + financial:view |
| GET | /exports/readings?type=transformer\|uc&transformer_id=&start=&end= | report:export |
| GET | /reports/comparative?month=YYYY-MM[&format=excel] | report:export |

### Assinatura (OWNER/ADMIN)
Acessível mesmo com trial expirado ou conta suspensa, para regularização.

| Método | Rota |
|---|---|
| GET | /billing |
| POST | /billing/activate `{plan, billing_type}` |
| PATCH | /billing/plan `{plan}` (upgrade cobra a diferença proporcional) |
| POST | /billing/cancel |
| GET | /billing/payments, /billing/proration, /billing/dunning |
| GET | /billing/payments/{id}/invoice, /boleto -> `{url}` |
| GET | /billing/payments/{id}/pix-qrcode -> `{qr_code_base64, pix_code, expires_at}` |

### Painel master (SUPER_ADMIN)
| Método | Rota |
|---|---|
| GET | /admin/tenants?status=&plan=&search=&page=&limit= (com usuários, trafos, UCs, último acesso) |
| GET | /admin/tenants/{id} (uso, limites, pagamentos, cobrança, ações administrativas) |
| GET | /admin/tenants/{id}/users |
| POST | /admin/tenants/{id}/activate `{plan}` |
| PATCH | /admin/tenants/{id}/plan `{plan}` |
| PATCH | /admin/tenants/{id}/status `{status, reason}` |
| POST | /admin/tenants/{id}/extend-trial `{days, reason}` |
| GET | /admin/billing/metrics?period=30d\|90d\|12m (MRR, ARR, churn, LTV, conversão de trial, NRR, por plano, série) |
| GET | /admin/audit |

## Jobs agendados

Rodam dentro da API (`SCHEDULER_ENABLED=true`, horário de Brasília), com lock
no Postgres para executar uma vez mesmo com várias máquinas:

| Job | Quando |
|---|---|
| trial_expiration | diário 00:15 |
| billing_metrics | diário 00:40 |
| balance_daily | diário 02:10 (mês corrente; até o dia 5 também o anterior) |
| dunning | diário 09:05 |
| webhook_retry | a cada 15 min |
| alert_retry | a cada 10 min |

`api cron` executa todos uma vez (para agendadores externos).
