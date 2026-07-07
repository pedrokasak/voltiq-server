# Voltiq Server - Month 1 Base Implementation

## Overview

Complete Go server implementation for Voltiq SaaS following AGENTS.md conventions:
- All variable names, functions, and database tables in **English**
- Unit tests for all calculation functions
- PostgreSQL + TimescaleDB for persistence
- JWT-based multitenancy authentication
- PRODIST Module 7 compliant calculations

## Project Structure

```
server/
├── cmd/api/main.go              # Application entry point
├── internal/
│   ├── api/                     # HTTP handlers and middleware
│   │   ├── auth.go             # JWT service
│   │   ├── auth_handler.go     # Login/register handlers
│   │   ├── middleware.go       # Auth middleware
│   │   ├── balance_handler.go  # Balance endpoints
│   │   ├── transformer_handler.go
│   │   ├── consuming_unit_handler.go
│   │   └── import_handler.go   # CSV upload handlers
│   ├── calc/                    # Calculation engine
│   │   ├── prodist_m7.go       # PRODIST M7 implementation
│   │   ├── prodist_m7_test.go  # Tests (100% coverage)
│   │   ├── balance.go          # Basic balance functions
│   │   └── balance_test.go     # Tests
│   ├── domain/                  # Domain entities
│   │   └── types.go            # All types in English
│   ├── ingestion/               # CSV parsing
│   │   ├── csv_parser.go
│   │   └── csv_parser_test.go
│   └── repository/              # Data access layer
│       ├── database.go
│       ├── tenant_repository.go
│       ├── user_repository.go
│       ├── transformer_repository.go
│       ├── consuming_unit_repository.go
│       ├── transformer_reading_repository.go
│       ├── consuming_unit_reading_repository.go
│       ├── balance_repository.go
│       └── import_repository.go
├── migrations/                  # Database migrations (English)
│   ├── 001_tenants_users.sql
│   ├── 002_electrical_network.sql
│   ├── 003_meter_readings.sql
│   ├── 004_balance_results.sql
│   └── 005_imports_alerts.sql
├── go.mod
├── go.sum
├── README.md
└── TESTS.md
```

## Implemented Features (Month 1)

### 1. CSV Upload and Parsing ✅
- Upload transformer readings (primary meter)
- Upload consuming unit readings
- Semicolon-separated CSV format
- Multiple date format support (ISO, BR)
- Error tracking per line

### 2. Transformer and UC Registration ✅
- CRUD for transformers
- CRUD for consuming units
- Link UCs to transformers
- Soft delete support

### 3. Basic Balance Calculation ✅
- Injected vs consumed energy
- Loss percentage calculation
- Status classification (NORMAL, WARNING, CRITICAL)
- PRODIST M7 technical loss calculation

### 4. PostgreSQL + TimescaleDB ✅
- Complete schema with migrations
- Hypertables for time-series data
- Automatic compression after 3 months
- Row-Level Security for multitenancy

### 5. Multitenancy Authentication ✅
- JWT-based authentication
- Tenant isolation via `app.tenant_id`
- Role-based access (ADMIN, ENGINEER, VIEWER)

## API Endpoints

### Authentication
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/login` | Login and get JWT token |
| POST | `/auth/register` | Register new user |

### Transformers
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/transformers` | Create transformer |
| GET | `/transformers` | List all transformers |
| GET | `/transformers/get?id=uuid` | Get by ID |
| PUT | `/transformers/update?id=uuid` | Update |
| DELETE | `/transformers/delete?id=uuid` | Soft delete |

### Consuming Units
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/consuming-units` | Create UC |
| GET | `/consuming-units` | List all UCs |
| GET | `/consuming-units/get?id=uuid` | Get by ID |
| GET | `/consuming-units/by-transformer?transformer_id=uuid` | List by transformer |
| PUT | `/consuming-units/update?id=uuid` | Update |
| DELETE | `/consuming-units/delete?id=uuid` | Soft delete |

### Imports
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/imports/transformers` | Upload transformer readings CSV |
| POST | `/imports/ucs` | Upload UC readings CSV |

### Balance
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/balance/calculate?transformer_id=uuid&period_start=YYYY-MM-DD&period_end=YYYY-MM-DD` | Calculate balance |
| GET | `/balance/list?transformer_id=uuid&period_start=...&period_end=...` | List balance history |
| GET | `/balance/latest?transformer_id=uuid` | Get latest balance |

## Database Schema (English)

### Tenants and Users
- `tenants` - Company/cooperative clients
- `users` - System users with roles

### Electrical Network
- `substations` - Electrical substations
- `transformers` - Distribution transformers
- `consuming_units` - Consumer units linked to transformers

### Meter Readings (TimescaleDB Hypertables)
- `transformer_readings` - Energy injected at transformer
- `consuming_unit_readings` - Consumption per UC

### Results
- `transformer_balance` - Balance calculation results
- `imports` - CSV import history
- `alerts` - Triggered alerts

## Environment Variables

```env
DATABASE_URL=postgres://user:pass@host:5432/energybalance?sslmode=disable
JWT_SECRET=your-secret-key-change-in-production
PORT=8080
LOG_LEVEL=info
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
# All tests
go test ./...

# With coverage
go test ./... -cover

# Specific packages
go test ./internal/calc/... -v
go test ./internal/ingestion/... -v

# Race detector
go test ./... -race
```

### Test Coverage
```
internal/calc        91.2%
internal/ingestion   84.9%
```

## CSV Format Examples

### Transformer Readings
```csv
transformer_id;reading_at;energy_kwh;demand_kw;power_factor
trafo-001;2024-01-01 00:00:00;1500.50;120.5;0.92
trafo-001;2024-01-02 00:00:00;1600.75;125.3;0.91
```

### Consuming Unit Readings
```csv
uc_id;reading_at;consumption_kwh
uc-001;2024-01-01 00:00:00;250.75
uc-001;2024-01-02 00:00:00;280.50
```

## PRODIST M7 Compliance

The calculation engine implements ANEEL PRODIST Module 7:

- **Section 6.1**: Energy balance calculation
  - Loss = Injected - Consumed
  - Loss % = (Loss / Injected) × 100

- **Section 6.2**: Transformer technical losses
  - PT_trafo = P0×T + Pcc×(Ic/In)²×T
  - P0: No-load losses (core)
  - Pcc: Load losses (winding)

- **Section 6.4**: Non-technical losses
  - PNT = Loss_total - Technical_loss

- **Status Classification**:
  - NORMAL: loss < 80% of limit
  - WARNING: 80% ≤ loss < 100% of limit
  - CRITICAL: loss ≥ 100% of limit

## Code Conventions (per AGENTS.md)

- All identifiers in **English**
- Packages in `snake_case` (not applicable for Go)
- Types in `PascalCase`
- Exported functions in `PascalCase`
- Errors always returned, never silenced
- All functions in `internal/calc/` have `_test.go`
- Business logic comments in Portuguese, generic code in English
- Database tables prefixed: `trafo_`, `reading_`, `user_`, etc.

## License

Proprietary — EnergyBalance SaaS
