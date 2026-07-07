package ingestion

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
)

// ReadingType represents the type of reading being imported
type ReadingType string

const (
	ReadingTypeTransformer ReadingType = "TRANSFORMER"
	ReadingTypeUC          ReadingType = "UC"
)

// CSVParser parses CSV files for meter readings
type CSVParser struct{}

// NewCSVParser creates a new CSVParser
func NewCSVParser() *CSVParser {
	return &CSVParser{}
}

// ParsedReading represents a parsed meter reading
type ParsedReading struct {
	TransformerID string
	UCID          string
	ReadingAt     time.Time
	EnergyKWh     float64
	DemandKW      *float64
	PowerFactor   *float64
}

// ParseResult contains successful parses and errors
type ParseResult struct {
	Success []*ParsedReading
	Errors  []ParseError
}

// ParseError represents a parsing error
type ParseError struct {
	Line    int
	Message string
}

// Parse reads and parses a CSV file
func (p *CSVParser) Parse(reader io.Reader, readingType ReadingType) (*ParseResult, error) {
	csvReader := csv.NewReader(reader)
	csvReader.Comma = ';'
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	columnIndex := make(map[string]int)
	for i, col := range header {
		columnIndex[col] = i
	}

	result := &ParseResult{
		Success: make([]*ParsedReading, 0),
		Errors:  make([]ParseError, 0),
	}

	lineNum := 1
	for {
		lineNum++
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, ParseError{
				Line:    lineNum,
				Message: fmt.Sprintf("failed to read line: %v", err),
			})
			continue
		}

		reading, parseErr := p.parseRecord(record, columnIndex, readingType, lineNum)
		if parseErr != nil {
			result.Errors = append(result.Errors, ParseError{
				Line:    lineNum,
				Message: parseErr.Error(),
			})
			continue
		}

		result.Success = append(result.Success, reading)
	}

	return result, nil
}

func (p *CSVParser) parseRecord(record []string, columnIndex map[string]int, readingType ReadingType, lineNum int) (*ParsedReading, error) {
	switch readingType {
	case ReadingTypeTransformer:
		return p.parseTransformerRecord(record, columnIndex, lineNum)
	case ReadingTypeUC:
		return p.parseUCRecord(record, columnIndex, lineNum)
	default:
		return nil, fmt.Errorf("unknown reading type: %s", readingType)
	}
}

func (p *CSVParser) parseTransformerRecord(record []string, columnIndex map[string]int, lineNum int) (*ParsedReading, error) {
	requiredCols := []string{"transformer_id", "reading_at", "energy_kwh"}
	for _, col := range requiredCols {
		if _, exists := columnIndex[col]; !exists {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}

	transformerID := record[columnIndex["transformer_id"]]
	if transformerID == "" {
		return nil, fmt.Errorf("empty transformer_id")
	}

	readingAt, err := p.parseTimestamp(record[columnIndex["reading_at"]])
	if err != nil {
		return nil, fmt.Errorf("invalid reading_at format: %w", err)
	}

	energyKWh, err := p.parseFloat(record[columnIndex["energy_kwh"]])
	if err != nil {
		return nil, fmt.Errorf("invalid energy_kwh: %w", err)
	}

	reading := &ParsedReading{
		TransformerID: transformerID,
		ReadingAt:     readingAt,
		EnergyKWh:     energyKWh,
	}

	if idx, exists := columnIndex["demand_kw"]; exists && record[idx] != "" {
		demand, err := p.parseFloat(record[idx])
		if err != nil {
			return nil, fmt.Errorf("invalid demand_kw at line %d: %w", lineNum, err)
		}
		reading.DemandKW = &demand
	}

	if idx, exists := columnIndex["power_factor"]; exists && record[idx] != "" {
		pf, err := p.parseFloat(record[idx])
		if err != nil {
			return nil, fmt.Errorf("invalid power_factor at line %d: %w", lineNum, err)
		}
		reading.PowerFactor = &pf
	}

	return reading, nil
}

func (p *CSVParser) parseUCRecord(record []string, columnIndex map[string]int, lineNum int) (*ParsedReading, error) {
	requiredCols := []string{"uc_id", "reading_at", "consumption_kwh"}
	for _, col := range requiredCols {
		if _, exists := columnIndex[col]; !exists {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}

	ucID := record[columnIndex["uc_id"]]
	if ucID == "" {
		return nil, fmt.Errorf("empty uc_id")
	}

	readingAt, err := p.parseTimestamp(record[columnIndex["reading_at"]])
	if err != nil {
		return nil, fmt.Errorf("invalid reading_at format: %w", err)
	}

	consumptionKWh, err := p.parseFloat(record[columnIndex["consumption_kwh"]])
	if err != nil {
		return nil, fmt.Errorf("invalid consumption_kwh: %w", err)
	}

	return &ParsedReading{
		UCID:      ucID,
		ReadingAt: readingAt,
		EnergyKWh: consumptionKWh,
	}, nil
}

func (p *CSVParser) parseTimestamp(value string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"02/01/2006 15:04:05",
		"02/01/2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", value)
}

func (p *CSVParser) parseFloat(value string) (float64, error) {
	if value == "" {
		return 0, nil
	}

	var result float64
	_, err := fmt.Sscanf(value, "%f", &result)
	if err != nil {
		return 0, err
	}

	return result, nil
}

// ToTransformerReadings converts parsed readings to domain TransformerReading entities
func (p *CSVParser) ToTransformerReadings(parsed []*ParsedReading, tenantID domain.UUID, importID *domain.UUID) []*domain.TransformerReading {
	readings := make([]*domain.TransformerReading, 0, len(parsed))

	for _, pr := range parsed {
		if pr.TransformerID == "" {
			continue
		}

		readings = append(readings, &domain.TransformerReading{
			ID:            domain.UUID(uuid.New().String()),
			TenantID:      tenantID,
			TransformerID: domain.UUID(pr.TransformerID),
			ReadingAt:     pr.ReadingAt,
			EnergyKWh:     pr.EnergyKWh,
			DemandKW:      pr.DemandKW,
			PowerFactor:   pr.PowerFactor,
			ImportID:      importID,
			CreatedAt:     time.Now(),
		})
	}

	return readings
}

// ToUCReadings converts parsed readings to domain UCReading entities
func (p *CSVParser) ToUCReadings(parsed []*ParsedReading, tenantID domain.UUID, importID *domain.UUID) []*domain.UCReading {
	readings := make([]*domain.UCReading, 0, len(parsed))

	for _, pr := range parsed {
		if pr.UCID == "" {
			continue
		}

		readings = append(readings, &domain.UCReading{
			ID:             domain.UUID(uuid.New().String()),
			TenantID:       tenantID,
			UCID:           domain.UUID(pr.UCID),
			ReadingAt:      pr.ReadingAt,
			ConsumptionKWh: pr.EnergyKWh,
			ImportID:       importID,
			CreatedAt:      time.Now(),
		})
	}

	return readings
}
