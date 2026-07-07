package ingestion_test

import (
	"strings"
	"testing"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/ingestion"
)

func TestCSVParser_ParseTransformerReadings(t *testing.T) {
	parser := ingestion.NewCSVParser()

	csvData := `transformer_id;reading_at;energy_kwh;demand_kw;power_factor
	trafo-001;2024-01-01 00:00:00;1500.50;120.5;0.92
	trafo-001;2024-01-02 00:00:00;1600.75;125.3;0.91
	trafo-002;2024-01-01 00:00:00;2000.00;150.0;0.95`

	reader := strings.NewReader(csvData)
	result, err := parser.Parse(reader, ingestion.ReadingTypeTransformer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Success) != 3 {
		t.Errorf("expected 3 successful parses, got %d", len(result.Success))
	}

	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}

	if result.Success[0].TransformerID != "trafo-001" {
		t.Errorf("expected transformer_id trafo-001, got %s", result.Success[0].TransformerID)
	}

	if result.Success[0].EnergyKWh != 1500.50 {
		t.Errorf("expected energy_kwh 1500.50, got %f", result.Success[0].EnergyKWh)
	}
}

func TestCSVParser_ParseUCReadings(t *testing.T) {
	parser := ingestion.NewCSVParser()

	csvData := `uc_id;reading_at;consumption_kwh
uc-001;2024-01-01 00:00:00;250.75
uc-001;2024-01-02 00:00:00;280.50
uc-002;2024-01-01 00:00:00;350.00`

	reader := strings.NewReader(csvData)
	result, err := parser.Parse(reader, ingestion.ReadingTypeUC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Success) != 3 {
		t.Errorf("expected 3 successful parses, got %d", len(result.Success))
	}

	if result.Success[0].UCID != "uc-001" {
		t.Errorf("expected uc_id uc-001, got %s", result.Success[0].UCID)
	}

	if result.Success[0].EnergyKWh != 250.75 {
		t.Errorf("expected consumption_kwh 250.75, got %f", result.Success[0].EnergyKWh)
	}
}

func TestCSVParser_ParseErrors(t *testing.T) {
	parser := ingestion.NewCSVParser()

	csvData := `transformer_id;reading_at;energy_kwh
trafo-001;2024-01-01 00:00:00;1500.50
;2024-01-02 00:00:00;1600.75
trafo-002;invalid-date;2000.00
trafo-003;2024-01-01 00:00:00;not-a-number`

	reader := strings.NewReader(csvData)
	result, err := parser.Parse(reader, ingestion.ReadingTypeTransformer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Success) != 1 {
		t.Errorf("expected 1 successful parse, got %d", len(result.Success))
	}

	if len(result.Errors) != 3 {
		t.Errorf("expected 3 errors, got %d", len(result.Errors))
	}
}

func TestCSVParser_ParseTimestamp(t *testing.T) {
	parser := ingestion.NewCSVParser()

	tests := []struct {
		name     string
		format   string
		expected time.Time
	}{
		{"ISO", "2024-01-01 15:04:05", time.Date(2024, 1, 1, 15, 4, 5, 0, time.UTC)},
		{"ISO_T", "2024-01-01T15:04:05", time.Date(2024, 1, 1, 15, 4, 5, 0, time.UTC)},
		{"BR", "01/01/2024 15:04:05", time.Date(2024, 1, 1, 15, 4, 5, 0, time.UTC)},
		{"BR_date", "01/01/2024", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csvData := "transformer_id;reading_at;energy_kwh\ntrafo-001;" + tt.format + ";100.0"
			reader := strings.NewReader(csvData)
			result, err := parser.Parse(reader, ingestion.ReadingTypeTransformer)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Success) != 1 {
				t.Fatalf("expected 1 successful parse, got %d", len(result.Success))
			}
			if !result.Success[0].ReadingAt.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result.Success[0].ReadingAt)
			}
		})
	}
}

func TestCSVParser_ToTransformerReadings(t *testing.T) {
	parser := ingestion.NewCSVParser()
	tenantID := domain.UUID("tenant-001")
	importID := domain.UUID("import-001")

	parsed := []*ingestion.ParsedReading{
		{
			TransformerID: "trafo-001",
			ReadingAt:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EnergyKWh:     1500.50,
			DemandKW:      ptrFloat(120.5),
			PowerFactor:   ptrFloat(0.92),
		},
		{
			TransformerID: "trafo-002",
			ReadingAt:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EnergyKWh:     2000.00,
		},
	}

	readings := parser.ToTransformerReadings(parsed, tenantID, &importID)

	if len(readings) != 2 {
		t.Errorf("expected 2 readings, got %d", len(readings))
	}

	if readings[0].TransformerID != "trafo-001" {
		t.Errorf("expected transformer_id trafo-001, got %s", readings[0].TransformerID)
	}

	if readings[0].TenantID != tenantID {
		t.Errorf("expected tenant_id %s, got %s", tenantID, readings[0].TenantID)
	}

	if readings[1].DemandKW != nil {
		t.Errorf("expected nil DemandKW for second reading, got %v", readings[1].DemandKW)
	}
}

func TestCSVParser_ToUCReadings(t *testing.T) {
	parser := ingestion.NewCSVParser()
	tenantID := domain.UUID("tenant-001")
	importID := domain.UUID("import-001")

	parsed := []*ingestion.ParsedReading{
		{
			UCID:      "uc-001",
			ReadingAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EnergyKWh: 250.75,
		},
		{
			UCID:      "uc-002",
			ReadingAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EnergyKWh: 350.00,
		},
	}

	readings := parser.ToUCReadings(parsed, tenantID, &importID)

	if len(readings) != 2 {
		t.Errorf("expected 2 readings, got %d", len(readings))
	}

	if readings[0].UCID != "uc-001" {
		t.Errorf("expected uc_id uc-001, got %s", readings[0].UCID)
	}

	if readings[0].ConsumptionKWh != 250.75 {
		t.Errorf("expected consumption_kwh 250.75, got %f", readings[0].ConsumptionKWh)
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}
