package ingestion

import (
	"fmt"
	"io"
	"time"
)

// ReadingType represents the type of reading being imported
type ReadingType string

const (
	ReadingTypeTransformer ReadingType = "TRANSFORMER"
	ReadingTypeUC          ReadingType = "UC"
)

// Kind returns the schema kind of the reading type.
func (t ReadingType) Kind() string {
	if t == ReadingTypeUC {
		return KindUCReadings
	}
	return KindTransformerReadings
}

// CSVParser parses meter reading files (CSV or XLSX)
type CSVParser struct {
	now func() time.Time
}

// NewCSVParser creates a new CSVParser
func NewCSVParser() *CSVParser {
	return &CSVParser{now: time.Now}
}

// ParsedReading represents a parsed meter reading. TransformerID and UCID
// hold the reference written in the file (code or UUID); the import use
// case resolves them to database IDs.
type ParsedReading struct {
	Line          int
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
	Mapping MappingReport
	Table   *Table
}

// ParseError represents a parsing error
type ParseError struct {
	Line    int    `json:"line"`
	Column  string `json:"column,omitempty"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

// Parse reads and parses a CSV stream (kept for callers that have a reader).
func (p *CSVParser) Parse(reader io.Reader, readingType ReadingType) (*ParseResult, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return p.ParseFile(content, "upload.csv", readingType, nil)
}

// ParseFile reads a CSV or XLSX file and parses its readings. explicit maps
// schema fields to file headers chosen by the user.
func (p *CSVParser) ParseFile(content []byte, filename string, readingType ReadingType, explicit map[string]string) (*ParseResult, error) {
	table, err := ReadTable(content, filename)
	if err != nil {
		return nil, err
	}
	return p.ParseTable(table, readingType, explicit)
}

// ParseTable validates and converts the rows of a table (RF02.2: colunas
// obrigatorias, tipos e valores negativos; RF02.3: erros linha a linha).
func (p *CSVParser) ParseTable(table *Table, readingType ReadingType, explicit map[string]string) (*ParseResult, error) {
	schema, _ := SchemaFor(readingType.Kind())
	mapping, report := schema.ResolveMapping(table.Headers, explicit)
	result := &ParseResult{
		Success: make([]*ParsedReading, 0, len(table.Rows)),
		Errors:  make([]ParseError, 0),
		Mapping: report,
		Table:   table,
	}
	if err := report.MissingError(); err != nil {
		return result, err
	}

	maxFuture := p.now().Add(48 * time.Hour)
	for i, row := range table.Rows {
		line := table.FirstDataLine + i
		if nonEmpty(row) == 0 {
			continue
		}
		reading, perr := p.parseRow(row, mapping, readingType, maxFuture)
		if perr != nil {
			perr.Line = line
			result.Errors = append(result.Errors, *perr)
			continue
		}
		reading.Line = line
		result.Success = append(result.Success, reading)
	}

	return result, nil
}

func (p *CSVParser) parseRow(row []string, m Mapping, readingType ReadingType, maxFuture time.Time) (*ParsedReading, *ParseError) {
	reading := &ParsedReading{}

	refField, energyField := "transformer", "energy_kwh"
	if readingType == ReadingTypeUC {
		refField, energyField = "uc", "consumption_kwh"
	}

	ref := m.Cell(row, refField)
	if ref == "" {
		return nil, &ParseError{Column: refField, Message: "identificador vazio"}
	}
	if readingType == ReadingTypeUC {
		reading.UCID = ref
	} else {
		reading.TransformerID = ref
	}

	rawAt := m.Cell(row, "reading_at")
	at, err := ParseTimestamp(rawAt)
	if err != nil {
		return nil, &ParseError{Column: "reading_at", Value: rawAt, Message: err.Error()}
	}
	if at.After(maxFuture) {
		return nil, &ParseError{Column: "reading_at", Value: rawAt, Message: "data no futuro"}
	}
	reading.ReadingAt = at

	rawEnergy := m.Cell(row, energyField)
	energy, err := ParseNumber(rawEnergy)
	if err != nil {
		return nil, &ParseError{Column: energyField, Value: rawEnergy, Message: err.Error()}
	}
	if energy < 0 {
		return nil, &ParseError{Column: energyField, Value: rawEnergy, Message: "valor negativo não é permitido"}
	}
	reading.EnergyKWh = energy

	if readingType == ReadingTypeTransformer {
		if raw := m.Cell(row, "demand_kw"); raw != "" {
			v, err := ParseNumber(raw)
			if err != nil {
				return nil, &ParseError{Column: "demand_kw", Value: raw, Message: err.Error()}
			}
			if v < 0 {
				return nil, &ParseError{Column: "demand_kw", Value: raw, Message: "valor negativo não é permitido"}
			}
			reading.DemandKW = &v
		}
		if raw := m.Cell(row, "power_factor"); raw != "" {
			v, err := ParseNumber(raw)
			if err != nil {
				return nil, &ParseError{Column: "power_factor", Value: raw, Message: err.Error()}
			}
			if v > 1 && v <= 100 { // informado em %
				v = v / 100
			}
			if v <= 0 || v > 1 {
				return nil, &ParseError{Column: "power_factor", Value: raw, Message: "fator de potência deve estar entre 0 e 1"}
			}
			reading.PowerFactor = &v
		}
	}

	return reading, nil
}
