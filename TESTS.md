# EnergyBalance Server Tests

## Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -cover

# Run specific package tests
go test ./internal/calc/... -v
go test ./internal/ingestion/... -v

# Run with race detector
go test ./... -race
```

## Test Coverage

### Calculation Engine (`internal/calc/`)
- `TestCalculateBalance_NormalCase` - Tests normal balance calculation
- `TestCalculateBalance_StatusNormal` - Tests NORMAL status classification
- `TestCalculateBalance_StatusWarning` - Tests WARNING status classification  
- `TestCalculateBalance_StatusCritical` - Tests CRITICAL status classification
- `TestCalculateBalance_TechnicalLossTrafo_PRODIST_M7` - Tests PRODIST M7 technical loss calculation
- `TestCalculateBalance_NonTechnicalLossNonNegative` - Tests non-technical loss normalization
- `TestCalculateBalance_SemUCs` - Tests balance with no consuming units
- `TestCalculateBalance_ErrorEnergyZero` - Tests validation for zero injected energy
- `TestCalculateBalance_ErrorPeriodZero` - Tests validation for zero period
- `TestCalculateBalance_ErrorNameplateData` - Tests validation for missing nameplate data
- `TestCalculateBalance_Batch` - Tests batch processing with goroutines
- `TestPeriodInHours` - Tests period conversion to hours
- `TestLoadFactor` - Tests load factor calculation
- `TestNominalCurrent_75kVA_220V` - Tests nominal current calculation
- `TestCalculateBasicBalance` - Tests basic balance calculation
- `TestCalculateLossPercentage` - Tests loss percentage calculation
- `TestDetermineBalanceStatus` - Tests balance status determination
- `TestCalculateTechnicalLossPRODIST` - Tests PRODIST technical loss calculation

### CSV Parser (`internal/ingestion/`)
- `TestCSVParser_ParseTransformerReadings` - Tests transformer readings CSV parsing
- `TestCSVParser_ParseUCReadings` - Tests UC readings CSV parsing
- `TestCSVParser_ParseErrors` - Tests error handling during parsing
- `TestCSVParser_ParseTimestamp` - Tests multiple timestamp format parsing
- `TestCSVParser_ToTransformerReadings` - Tests conversion to domain entities
- `TestCSVParser_ToUCReadings` - Tests conversion to domain entities

## Code Conventions

All tests follow these conventions:
- Test files in `*_test.go` format
- Table-driven tests where applicable
- Clear error messages with expected vs actual values
- Test coverage for all exported functions in `internal/calc/`

## Coverage Report

```
internal/calc        91.2%
internal/ingestion   84.9%
```

## PRODIST M7 Compliance

The calculation engine (`internal/calc/prodist_m7.go`) implements:
- Section 6.1: Energy balance calculation
- Section 6.2: Transformer technical losses (PT_trafo = P0×T + Pcc×(Ic/In)²×T)
- Section 6.4: Non-technical loss estimation
- Regulatory status classification (NORMAL < 80%, WARNING 80-100%, CRITICAL ≥ 100%)
