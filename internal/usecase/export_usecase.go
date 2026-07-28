package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// ExportFormat represents the export format requested by the client
type ExportFormat string

const (
	ExportFormatExcel ExportFormat = "excel"
	ExportFormatPDF   ExportFormat = "pdf"
)

// ErrExportInvalidFormat indicates an unsupported export format
var ErrExportInvalidFormat = errors.New("invalid export format: must be pdf or excel")

// ErrExportTransformerNotFound indicates the transformer does not exist
var ErrExportTransformerNotFound = errors.New("transformer not found")

// ErrExportNoBalances indicates there are no balances to export in the period
var ErrExportNoBalances = errors.New("no balances found for the requested period")

// ErrExportGeneration indicates the export generation failed
var ErrExportGeneration = errors.New("failed to generate export")

// ExportUseCase generates export artifacts (Excel/PDF) of transformer balances
type ExportUseCase struct {
	balanceRepo     *repository.BalanceRepository
	transformerRepo *repository.TransformerRepository
}

// NewExportUseCase creates a new ExportUseCase
func NewExportUseCase(
	balanceRepo *repository.BalanceRepository,
	transformerRepo *repository.TransformerRepository,
) *ExportUseCase {
	return &ExportUseCase{
		balanceRepo:     balanceRepo,
		transformerRepo: transformerRepo,
	}
}

// ExportBalance generates an export artifact for the latest balances of a transformer.
// The default period is the last 90 days. It returns the bytes, the MIME type and a
// suggested filename extension.
func (uc *ExportUseCase) ExportBalance(
	ctx context.Context,
	transformerID domain.UUID,
	format ExportFormat,
) ([]byte, string, string, error) {
	// Validate transformer exists (and belongs to the platform)
	t, err := uc.transformerRepo.GetByID(ctx, transformerID)
	if err != nil {
		return nil, "", "", err
	}
	if t == nil {
		return nil, "", "", ErrExportTransformerNotFound
	}

	// Default period: last 90 days
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -90)

	balances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, transformerID, start, end)
	if err != nil {
		return nil, "", "", err
	}
	if len(balances) == 0 {
		return nil, "", "", ErrExportNoBalances
	}

	switch format {
	case ExportFormatExcel:
		buf, err := buildBalanceXLSX(transformerID, t.Code, balances)
		if err != nil {
			return nil, "", "", fmt.Errorf("%w: %v", ErrExportGeneration, err)
		}
		return buf.Bytes(),
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"xlsx",
			nil
	case ExportFormatPDF:
		buf, err := buildBalancePDF(transformerID, t.Code, balances)
		if err != nil {
			return nil, "", "", fmt.Errorf("%w: %v", ErrExportGeneration, err)
		}
		return buf.Bytes(), "application/pdf", "pdf", nil
	default:
		return nil, "", "", ErrExportInvalidFormat
	}
}

// ParseExportFormat normalizes the format string from the query param
func ParseExportFormat(s string) ExportFormat {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "pdf" {
		return ExportFormatPDF
	}
	return ExportFormatExcel
}

// buildBalanceXLSX builds an in-memory .xlsx with one sheet containing the balance rows
func buildBalanceXLSX(
	transformerID domain.UUID,
	transformerCode string,
	balances []*domain.TransformerBalance,
) (*bytes.Buffer, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Balanço"
	if _, err := f.NewSheet(sheet); err != nil {
		return nil, err
	}

	headers := []string{
		"Período início",
		"Período fim",
		"Energia injetada (kWh)",
		"Consumo total (kWh)",
		"Perda (kWh)",
		"Perda (%)",
		"Status",
		"UCs",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, err
		}
	}

	for rowIdx, b := range balances {
		row := rowIdx + 2
		values := []any{
			b.PeriodStart.Format("2006-01-02"),
			b.PeriodEnd.Format("2006-01-02"),
			b.EnergyInjectedKWh,
			b.TotalConsumptionKWh,
			b.LossKWh,
			b.LossPct,
			string(b.Status),
			b.UCCount,
		}
		for colIdx, v := range values {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, row)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return nil, err
			}
		}
	}

	// Delete the default Sheet1
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, err
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// buildBalancePDF builds an in-memory PDF with a simple tabular layout
func buildBalancePDF(
	transformerID domain.UUID,
	transformerCode string,
	balances []*domain.TransformerBalance,
) (*bytes.Buffer, error) {
	m := maroto.New()

	// Title
	m.AddRows(text.NewRow(20, fmt.Sprintf("Relatório de Balanço — Transformador %s", transformerCode),
		props.Text{Size: 14, Style: fontstyle.Bold, Align: align.Center}))

	// Subtitle (period)
	m.AddRows(text.NewRow(15, fmt.Sprintf("Período: %s a %s",
		balances[0].PeriodEnd.Format("2006-01-02"),
		balances[len(balances)-1].PeriodStart.Format("2006-01-02"),
	), props.Text{Size: 10, Align: align.Center}))

	// One line per balance
	for _, b := range balances {
		line := fmt.Sprintf(
			"%s → %s | Inj=%.2f kWh | Cons=%.2f kWh | Perda=%.2f kWh (%.2f%%) | %s | UCs=%d",
			b.PeriodStart.Format("2006-01-02"),
			b.PeriodEnd.Format("2006-01-02"),
			b.EnergyInjectedKWh,
			b.TotalConsumptionKWh,
			b.LossKWh,
			b.LossPct,
			b.Status,
			b.UCCount,
		)
		m.AddRows(text.NewRow(10, line, props.Text{Size: 9, Align: align.Left}))
	}

	doc, err := m.Generate()
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(doc.GetBytes()), nil
}
