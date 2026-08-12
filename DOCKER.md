# 🐳 Deploy com Docker

Guia completo para rodar a API Voltiq usando Docker e Docker Compose.

---

## ✅ Problema Resolvido

**Erro anterior:** `go.mod requires go >= 1.25.0`

**Solução:** Atualizamos as dependências para versões compatíveis com Go 1.22:

```diff
- go 1.25.0
+ go 1.22

- github.com/jackc/pgx/v5 v5.10.0
+ github.com/jackc/pgx/v5 v5.5.5

- golang.org/x/crypto v0.53.0
+ golang.org/x/crypto v0.21.0
```

---

## 🚀 Quick Start

### 1. Construir Imagem

```bash
cd server

# Build da imagem
docker build -t voltiq-server:latest .

# Ou usar docker-compose
docker-compose build
```

### 2. Rodar com Docker Compose

```bash
# Subir todos os serviços (API + PostgreSQL + TimescaleDB)
docker-compose up -d

# Ver logs
docker-compose logs -f api

# Ver status
docker-compose ps
```

### 3. Rodar Migrations

```bash
# Rodar migrations no banco
docker-compose run migrate
```

### 4. Testar API

```bash
# Health check
curl http://localhost:8080/health

# Expected response:
# {"success":true,"data":{"status":"healthy",...}}
```

---

## 📦 Serviços

| Serviço | Porta | Descrição |
|---------|-------|-----------|
| **api** | 8080 | API Go (voltiq-server:latest) |
| **db** | 5432 | PostgreSQL 16 + TimescaleDB |
| **migrate** | - | Migrations (one-off) |

---

## 🔧 Comandos Úteis

### Ver Logs

```bash
# Todos os logs
docker-compose logs -f

# Apenas API
docker-compose logs -f api

# Apenas Database
docker-compose logs -f db
```

### Parar Serviços

```bash
# Parar (mantém volumes)
docker-compose down

# Parar e remover volumes
docker-compose down -v
```

### Rebuild

```bash
# Rebuild completo
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

### Acessar Container

```bash
# API shell
docker exec -it voltiq-sw-api sh

# Database shell
docker exec -it voltiq-sw-db psql -U postgres -d voltiq-sw
```

---

## 🗄️ Banco de Dados

### Connection String

```
postgresql://postgres:postgres@localhost:5432/voltiq-sw?sslmode=disable
```

### Rodar Migrations Manualmente

```bash
# Via Docker
docker-compose run migrate

# Via psql
psql -U postgres -d voltiq-sw -f server/migrations/001_tenants_users.sql
```

### Verificar Tables

```bash
docker exec -it voltiq-sw-db psql -U postgres -d voltiq-sw -c "\dt"
```

---

## 🌍 Variáveis de Ambiente

Crie o arquivo `server/.env`:

```env
# Database
DATABASE_URL=postgres://postgres:postgres@db:5432/voltiq-sw?sslmode=disable

# JWT
JWT_SECRET=your-secret-key-change-in-production
JWT_EXPIRATION_HOURS=8
REFRESH_TOKEN_EXPIRATION_DAYS=7

# Server
PORT=8080

# CORS
CORS_ALLOWED_ORIGINS=http://localhost:8089,http://localhost:3000

# Rate Limiting
RATE_LIMIT_REQUESTS_PER_MINUTE=60
RATE_LIMIT_BURST=30
```

---

## 🏗️ Build Manual (sem docker-compose)

```bash
# 1. Build da imagem
docker build -t voltiq-server:latest .

# 2. Rodar container
docker run -d \
  --name voltiq-api \
  -p 8080:8080 \
  --env-file .env \
  voltiq-server:latest

# 3. Ver logs
docker logs -f voltiq-api

# 4. Health check
curl http://localhost:8080/health
```

---

## 🔄 Update/Redeploy

```bash
# 1. Parar container atual
docker-compose down

# 2. Rebuild da imagem
docker-compose build --no-cache

# 3. Subir novamente
docker-compose up -d

# 4. Verificar
docker-compose ps
docker-compose logs -f api
```

---

## 🐛 Troubleshooting

### Erro: "database does not exist"

```bash
# Rodar migrations
docker-compose run migrate
```

### Erro: "connection refused"

```bash
# Verificar se banco está saudável
docker-compose ps
docker-compose logs db

# Aguardar health check
sleep 10
```

### Erro: "port already in use"

```bash
# Mapear para porta diferente
docker-compose up -d --scale api=0
docker run -d -p 8081:8080 voltiq-server:latest
```

### API não responde

```bash
# Ver logs
docker-compose logs api

# Restart
docker-compose restart api

# Rebuild
docker-compose build api
docker-compose up -d api
```

---

## 📊 Monitoramento

### Health Check

```bash
# Via curl
curl http://localhost:8080/health

# Via docker exec
docker exec voltiq-sw-api wget -q --spider http://localhost:8080/health
```

### Metrics (Prometheus)

```bash
curl http://localhost:8080/metrics
```

### Resource Usage

```bash
docker stats voltiq-sw-api voltiq-sw-db
```

---

## 🎯 Produção

### Security Best Practices

1. **Mudar JWT_SECRET** no `.env`
2. **Usar HTTPS** via reverse proxy (nginx/traefik)
3. **Não expor porta 5432** publicamente
4. **Usar volumes named** para dados persistentes
5. **Habilitar logs** em produção

### Exemplo docker-compose.prod.yml

```yaml
version: '3.9'

services:
  api:
    image: voltiq-server:latest
    environment:
      - JWT_SECRET=${JWT_SECRET}
      - DATABASE_URL=${DATABASE_URL}
    networks:
      - backend
    restart: always
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: '1.0'
          memory: 512M

  db:
    image: timescale/timescaledb:latest-pg16
    environment:
      - POSTGRES_USER=${DB_USER}
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    networks:
      - backend
    restart: always

volumes:
  pgdata:

networks:
  backend:
    driver: overlay
```

---

## 📚 Referências

- [Dockerfile](./Dockerfile)
- [docker-compose.yml](./docker-compose.yml)
- [Documentação API](../docs/API.md)
- [Postman Collection](../docs/postman/README.md)

---

**Status:** ✅ Build bem-sucedido  
**Imagem:** `voltiq-server:latest`  
**Go Version:** 1.22.12  
**PostgreSQL:** 16 + TimescaleDB
