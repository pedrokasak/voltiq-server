# EnergyBalance Server

Go server implementation for EnergyBalance SaaS — energy balance calculation and technical loss analysis for distribution transformers per ANEEL PRODIST Module 7.

## Project Structure

```
server/
├── cmd/
│   └── api/              # Application entry point
├── internal/
│   ├── api/              # HTTP handlers and middleware
│   ├── calc/             # PRODIST M7 calculation engine
│   ├── domain/           # Domain entities and types
│   ├── ingestion/        # CSV parsers and data import
│   └── repository/       # Database access layer
├── migrations/           # SQL migrations (PostgreSQL + TimescaleDB)
├── go.mod
└── go.sum
```

## Month 1 — Base Features

### ✅ Completed

1. **CSV Upload and Parsing**
   - Upload transformer readings (primary meter)
   - Upload consuming unit readings
   - Semicolon-separated CSV format support
   - Multiple date format parsing

2. **Transformer and UC Registration**
   - CRUD for transformers (trafos)
   - CRUD for consuming units (UCs)
   - Link UCs to transformers

3. **Basic Balance Calculation**
   - Injected vs consumed energy
   - Loss percentage calculation
   - Status classification (NORMAL, ATENCAO, CRITICO)

4. **PostgreSQL + TimescaleDB Persistence**
   - Full schema with migrations
   - Hypertables for time-series data
   - Automatic compression after 3 months
   - Row-Level Security for multitenancy

5. **Multitenancy Authentication**
   - JWT-based authentication
   - Tenant isolation via database policies
   - Role-based access control (ADMIN, ENGINEER, VIEWER)

## Environment Variables

```env
DATABASE_URL=postgres://user:pass@host:5432/energybalance?sslmode=disable
JWT_SECRET=your-secret-key
PORT=8080
```

## API Endpoints

### Authentication
- `POST /auth/login` — Login and get JWT token
- `POST /auth/register` — Register new user

### Transformers
- `POST /transformers` — Create transformer
- `GET /transformers` — List all transformers (tenant-scoped)
- `GET /transformers/get?id=uuid` — Get transformer by ID
- `PUT /transformers/update?id=uuid` — Update transformer
- `DELETE /transformers/delete?id=uuid` — Soft delete transformer

### Consuming Units
- `POST /consuming-units` — Create UC
- `GET /consuming-units` — List all UCs (tenant-scoped)
- `GET /consuming-units/get?id=uuid` — Get UC by ID
- `GET /consuming-units/by-transformer?transformer_id=uuid` — List UCs by transformer
- `PUT /consuming-units/update?id=uuid` — Update UC
- `DELETE /consuming-units/delete?id=uuid` — Soft delete UC

### Imports
- `POST /imports/transformers` — Upload transformer readings CSV
- `POST /imports/ucs` — Upload UC readings CSV

### Balance Calculation
- `POST /balance/calculate?trafo_id=uuid&periodo_inicio=YYYY-MM-DD&periodo_fim=YYYY-MM-DD` — Calculate balance for transformer
- `POST /balance/recalculate-all` — Recalculate all transformers
- `GET /balance/list?trafo_id=uuid&periodo_inicio=YYYY-MM-DD&periodo_fim=YYYY-MM-DD` — List balance history
- `GET /balance/latest?trafo_id=uuid` — Get latest balance result

## CSV Format

### Transformer Readings
```csv
trafo_id;reading_at;energy_kwh;demand_kw;power_factor
trafo-001;2024-01-01 00:00:00;1500.50;120.5;0.92
```

### Consuming Unit Readings
```csv
uc_id;reading_at;consumption_kwh
uc-001;2024-01-01 00:00:00;250.75
```

## Database Migrations

Run migrations in order:

```bash
psql $DATABASE_URL -f migrations/001_tenants_users.sql
psql $DATABASE_URL -f migrations/002_electrical_network.sql
psql $DATABASE_URL -f migrations/003_meter_readings.sql
psql $DATABASE_URL -f migrations/004_balance_results.sql
psql $DATABASE_URL -f migrations/005_imports_alerts.sql
```

## Running

```bash
# Install dependencies
go mod tidy

# Build
go build ./cmd/api

# Run
./api

# Or directly
go run ./cmd/api
```

## Testing

```bash
# Run all tests
go test ./...

# Run calculation tests with coverage
go test -cover ./internal/calc/...
```

## License

Proprietary — EnergyBalance SaaS
