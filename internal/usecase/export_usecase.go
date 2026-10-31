package usecase

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/xuri/excelize/v2"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// ExportFormat represents the export format requested by the client
type ExportFormat string

const (
	ExportFormatExcel ExportFormat = "excel"
	ExportFormatPDF   ExportFormat = "pdf"
	ExportFormatCSV   ExportFormat = "csv"
)

var (
	// ErrExportInvalidFormat indicates an unsupported export format
	ErrExportInvalidFormat = errors.New("invalid export format: must be pdf, excel or csv")
	// ErrExportTransformerNotFound indicates the transformer does not exist
	ErrExportTransformerNotFound = errors.New("transformer not found")
	// ErrExportNoBalances indicates there are no balances to export in the period
	ErrExportNoBalances = errors.New("no balances found for the requested period")
	// ErrExportGeneration indicates the export generation failed
	ErrExportGeneration = errors.New("failed to generate export")
)

const (
	mimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimePDF  = "application/pdf"
	mimeCSV  = "text/csv; charset=utf-8"
)

// ExportFile is a generated file.
type ExportFile struct {
	Content     []byte
	ContentType string
	Filename    string
}

// ExportUseCase generates export artifacts (RF06)
type ExportUseCase struct {
	balanceRepo     *repository.BalanceRepository
	transformerRepo *repository.TransformerRepository
	tenantRepo      *repository.TenantRepository
	trafoReadings   *repository.TransformerReadingRepository
	ucReadings      *repository.UCReadingRepository
	reports         *ReportUseCase
}

// NewExportUseCase creates a new ExportUseCase
func NewExportUseCase(
	balanceRepo *repository.BalanceRepository,
	transformerRepo *repository.TransformerRepository,
	tenantRepo *repository.TenantRepository,
	trafoReadings *repository.TransformerReadingRepository,
	ucReadings *repository.UCReadingRepository,
	reports *ReportUseCase,
) *ExportUseCase {
	return &ExportUseCase{
		balanceRepo:     balanceRepo,
		transformerRepo: transformerRepo,
		tenantRepo:      tenantRepo,
		trafoReadings:   trafoReadings,
		ucReadings:      ucReadings,
		reports:         reports,
	}
}

// ParseExportFormat normalizes the format string from the query param
func ParseExportFormat(s string) ExportFormat {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pdf":
		return ExportFormatPDF
	case "csv":
		return ExportFormatCSV
	}
	return ExportFormatExcel
}

func (uc *ExportUseCase) tenantName(ctx context.Context, id domain.UUID) string {
	if uc.tenantRepo == nil {
		return ""
	}
	if t, err := uc.tenantRepo.GetByID(ctx, id); err == nil && t != nil {
		return t.Name
	}
	return ""
}

// ExportBalance exporta os balancos de um trafo no periodo (RF06.1: relatorio
// mensal em PDF; tambem Excel e CSV). O padrao sao os ultimos 12 meses.
func (uc *ExportUseCase) ExportBalance(ctx context.Context, transformerID domain.UUID, format ExportFormat, start, end time.Time) (*ExportFile, error) {
	t, err := uc.transformerRepo.GetByID(ctx, transformerID)
	if err != nil || t == nil {
		return nil, ErrExportTransformerNotFound
	}
	if end.IsZero() {
		end = time.Now()
	}
	if start.IsZero() {
		start = end.AddDate(-1, 0, 0)
	}
	balances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, transformerID, start, end)
	if err != nil {
		return nil, err
	}
	if len(balances) == 0 {
		return nil, ErrExportNoBalances
	}
	// mais antigo primeiro
	sort.Slice(balances, func(i, j int) bool { return balances[i].PeriodStart.Before(balances[j].PeriodStart) })

	base := fmt.Sprintf("balanco_%s_%s", fileSafe(t.Code), time.Now().Format("20060102"))
	switch format {
	case ExportFormatPDF:
		buf, err := buildBalancePDF(uc.tenantName(ctx, t.TenantID), t, balances)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrExportGeneration, err)
		}
		return &ExportFile{Content: buf, ContentType: mimePDF, Filename: base + ".pdf"}, nil
	case ExportFormatCSV:
		return &ExportFile{Content: balancesCSV(t, balances), ContentType: mimeCSV, Filename: base + ".csv"}, nil
	case ExportFormatExcel:
		buf, err := buildBalanceXLSX(t, balances)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrExportGeneration, err)
		}
		return &ExportFile{Content: buf, ContentType: mimeXLSX, Filename: base + ".xlsx"}, nil
	}
	return nil, ErrExportInvalidFormat
}

func fileSafe(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func fmtKWh(v float64) string { return formatBR(v, 2) }

// formatBR formats a number with Brazilian separators (1.234,56).
func formatBR(v float64, decimals int) string {
	s := fmt.Sprintf("%.*f", decimals, v)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if frac != "" {
		out += "," + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

func optKWh(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmtKWh(*v)
}

func statusPT(s domain.BalanceStatus) string {
	switch s {
	case domain.BalanceStatusCritical:
		return "CRÍTICO"
	case domain.BalanceStatusWarning:
		return "ATENÇÃO"
	}
	return "NORMAL"
}

var (
	pdfTitle  = props.Text{Size: 14, Style: fontstyle.Bold, Align: align.Left}
	pdfSub    = props.Text{Size: 9, Align: align.Left, Color: &props.Color{Red: 90, Green: 90, Blue: 90}}
	pdfLabel  = props.Text{Size: 8, Align: align.Left, Color: &props.Color{Red: 100, Green: 100, Blue: 100}}
	pdfValue  = props.Text{Size: 10, Style: fontstyle.Bold, Align: align.Left}
	pdfHeader = props.Text{Size: 7, Style: fontstyle.Bold, Align: align.Center}
	pdfCell   = props.Text{Size: 7, Align: align.Center}
	pdfNote   = props.Text{Size: 7, Align: align.Left, Color: &props.Color{Red: 110, Green: 110, Blue: 110}}
)

// buildBalancePDF gera o relatorio de balanco do trafo: identificacao, dados
// de placa, resumo do periodo e tabela mensal com perdas tecnicas e nao
// tecnicas (PRODIST Modulo 7, Secoes 6.1, 6.2 e 6.4).
func buildBalancePDF(tenantName string, t *domain.Transformer, balances []*domain.TransformerBalance) ([]byte, error) {
	cfg := config.NewBuilder().
		WithLeftMargin(12).WithRightMargin(12).WithTopMargin(12).
		WithPageNumber(props.PageNumber{Pattern: "Página {current} de {total}", Place: props.Bottom, Size: 7}).
		WithTitle("Relatório de Balanço de Energia - "+t.Code, true).
		Build()
	m := maroto.New(cfg)

	first, last := balances[0], balances[len(balances)-1]
	m.AddRows(
		text.NewRow(8, "Relatório de Balanço de Energia e Perdas", pdfTitle),
		text.NewRow(5, strings.TrimSpace(tenantName+" · Transformador "+t.Code), pdfSub),
		text.NewRow(5, fmt.Sprintf("Período: %s a %s · Emitido em %s",
			first.PeriodStart.Format("02/01/2006"), last.PeriodEnd.Format("02/01/2006"), time.Now().Format("02/01/2006 15:04")), pdfSub),
		line.NewRow(4),
	)

	kv := func(label, value string) core.Col {
		return col.New(3).Add(text.New(label, pdfLabel), text.New(value, props.Text{Top: 4, Size: 10, Style: fontstyle.Bold}))
	}
	limit := t.LossLimitPctValue()
	m.AddRows(
		row.New(12).Add(
			kv("Potência nominal", formatBR(t.PowerKVA, 1)+" kVA"),
			kv("Tensões", fmt.Sprintf("%s kV / %s V", formatBR(t.PrimaryVoltageKV, 1), formatBR(t.SecondaryVoltageV, 0))),
			kv("P0 / Pcc (kW)", optKWh(t.CoreLossKW)+" / "+optKWh(t.WindingLossKW)),
			kv("Limite ANEEL", formatBR(limit, 1)+"%"),
		),
	)

	var inj, cons, loss, tech, pnt float64
	critical := 0
	for _, b := range balances {
		inj += b.EnergyInjectedKWh
		cons += b.TotalConsumptionKWh
		loss += b.LossKWh
		if b.TechnicalLossKWh != nil {
			tech += *b.TechnicalLossKWh
		}
		if b.NonTechnicalLossKWh != nil {
			pnt += *b.NonTechnicalLossKWh
		}
		if b.Status == domain.BalanceStatusCritical {
			critical++
		}
	}
	lossPct := 0.0
	if inj > 0 {
		lossPct = loss / inj * 100
	}
	m.AddRows(
		text.NewRow(8, "Resumo do período", props.Text{Top: 3, Size: 10, Style: fontstyle.Bold}),
		row.New(12).Add(
			kv("Energia injetada", fmtKWh(inj)+" kWh"),
			kv("Energia faturada", fmtKWh(cons)+" kWh"),
			kv("Perda total", fmtKWh(loss)+" kWh"),
			kv("Perda (%)", formatBR(lossPct, 2)+"%"),
		),
		row.New(12).Add(
			kv("Perda técnica", fmtKWh(tech)+" kWh"),
			kv("Perda não técnica", fmtKWh(pnt)+" kWh"),
			kv("Meses acima do limite", fmt.Sprintf("%d de %d", critical, len(balances))),
			kv("Status atual", statusPT(last.Status)),
		),
		line.NewRow(4),
	)

	headers := []string{"Período", "Injetada (kWh)", "Faturada (kWh)", "Perda (kWh)", "Perda (%)", "Técnica (kWh)", "Não técnica (kWh)", "UCs", "Status"}
	sizes := []int{2, 1, 1, 1, 1, 2, 2, 1, 1}
	hdr := row.New(7)
	for i, h := range headers {
		hdr.Add(text.NewCol(sizes[i], h, pdfHeader))
	}
	m.AddRows(hdr)
	for _, b := range balances {
		values := []string{
			b.PeriodStart.Format("01/2006"), fmtKWh(b.EnergyInjectedKWh), fmtKWh(b.TotalConsumptionKWh),
			fmtKWh(b.LossKWh), formatBR(b.LossPct, 2), optKWh(b.TechnicalLossKWh), optKWh(b.NonTechnicalLossKWh),
			fmt.Sprint(b.UCCount), statusPT(b.Status),
		}
		r := row.New(6)
		for i, v := range values {
			r.Add(text.NewCol(sizes[i], v, pdfCell))
		}
		m.AddRows(r)
	}

	m.AddRows(
		line.NewRow(6),
		text.NewAutoRow("Metodologia: balanço = energia injetada - Σ consumo das UCs (PRODIST Módulo 7, Seção 6.1). "+
			"Perda técnica no transformador = P0 × T + Pcc × (Ic/In)² × T (Seção 6.2); sem corrente medida considera-se 50% de carregamento. "+
			"Perda não técnica = perda total - perda técnica (Seção 6.4). Status: NORMAL abaixo de 80% do limite, ATENÇÃO entre 80% e 100%, CRÍTICO no limite ou acima.", pdfNote),
	)

	doc, err := m.Generate()
	if err != nil {
		return nil, err
	}
	return doc.GetBytes(), nil
}

func newSheet(f *excelize.File, name string, headers []string) (int, error) {
	idx, err := f.NewSheet(name)
	if err != nil {
		return 0, err
	}
	style, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#E8F0EA"}, Pattern: 1}})
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(name, cell, h); err != nil {
			return 0, err
		}
		_ = f.SetCellStyle(name, cell, cell, style)
		colName, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(name, colName, colName, 18)
	}
	_ = f.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	return idx, nil
}

func setRow(f *excelize.File, sheet string, rowNum int, values []any) error {
	cell, _ := excelize.CoordinatesToCellName(1, rowNum)
	return f.SetSheetRow(sheet, cell, &values)
}

func optNum(v *float64) any {
	if v == nil {
		return ""
	}
	return *v
}

// buildBalanceXLSX builds the transformer balance workbook.
func buildBalanceXLSX(t *domain.Transformer, balances []*domain.TransformerBalance) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Balanço"
	if _, err := newSheet(f, sheet, []string{"Transformador", "Período início", "Período fim", "Energia injetada (kWh)",
		"Consumo total (kWh)", "Perda (kWh)", "Perda (%)", "Perda técnica (kWh)", "Perda não técnica (kWh)",
		"Limite ANEEL (%)", "Status", "UCs", "Calculado em"}); err != nil {
		return nil, err
	}
	limit := t.LossLimitPctValue()
	for i, b := range balances {
		if err := setRow(f, sheet, i+2, []any{t.Code, b.PeriodStart.Format("2006-01-02"), b.PeriodEnd.Format("2006-01-02"),
			b.EnergyInjectedKWh, b.TotalConsumptionKWh, b.LossKWh, b.LossPct, optNum(b.TechnicalLossKWh),
			optNum(b.NonTechnicalLossKWh), limit, statusPT(b.Status), b.UCCount, b.CalculatedAt.Format("2006-01-02 15:04")}); err != nil {
			return nil, err
		}
	}
	_ = f.DeleteSheet("Sheet1")
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func balancesCSV(t *domain.Transformer, balances []*domain.TransformerBalance) []byte {
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	_ = w.Write([]string{"transformador", "periodo_inicio", "periodo_fim", "energia_injetada_kwh", "consumo_kwh",
		"perda_kwh", "perda_pct", "perda_tecnica_kwh", "perda_nao_tecnica_kwh", "status", "ucs"})
	for _, b := range balances {
		_ = w.Write([]string{t.Code, b.PeriodStart.Format("2006-01-02"), b.PeriodEnd.Format("2006-01-02"),
			formatBR(b.EnergyInjectedKWh, 4), formatBR(b.TotalConsumptionKWh, 4), formatBR(b.LossKWh, 4),
			formatBR(b.LossPct, 4), optCSV(b.TechnicalLossKWh), optCSV(b.NonTechnicalLossKWh), string(b.Status), fmt.Sprint(b.UCCount)})
	}
	w.Flush()
	return buf.Bytes()
}

func optCSV(v *float64) string {
	if v == nil {
		return ""
	}
	return formatBR(*v, 4)
}

// ExportANEEL gera o relatorio consolidado de perdas do tenant em Excel
// (RF06.2): resumo, balanco por trafo e mes, consolidacao por subestacao e
// evolucao mensal.
func (uc *ExportUseCase) ExportANEEL(ctx context.Context, tenantID domain.UUID, start, end time.Time) (*ExportFile, error) {
	rows, err := uc.balanceRepo.ListRowsByTenantPeriod(ctx, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrExportNoBalances
	}

	f := excelize.NewFile()
	defer f.Close()

	// Resumo
	var inj, cons, loss, tech, pnt float64
	trafos := map[domain.UUID]bool{}
	status := map[domain.BalanceStatus]int{}
	for _, r := range rows {
		inj += r.EnergyInjectedKWh
		cons += r.TotalConsumptionKWh
		loss += r.LossKWh
		if r.TechnicalLossKWh != nil {
			tech += *r.TechnicalLossKWh
		}
		if r.NonTechnicalLossKWh != nil {
			pnt += *r.NonTechnicalLossKWh
		}
		trafos[r.TransformerID] = true
		status[r.Status]++
	}
	lossPct := 0.0
	if inj > 0 {
		lossPct = loss / inj * 100
	}
	summary := "Resumo"
	if _, err := newSheet(f, summary, []string{"Indicador", "Valor"}); err != nil {
		return nil, err
	}
	items := [][]any{
		{"Distribuidora / cooperativa", uc.tenantName(ctx, tenantID)},
		{"Período", start.Format("02/01/2006") + " a " + end.Format("02/01/2006")},
		{"Transformadores com balanço", len(trafos)},
		{"Balanços no período", len(rows)},
		{"Energia injetada (kWh)", inj},
		{"Energia faturada (kWh)", cons},
		{"Perda total (kWh)", loss},
		{"Perda total (%)", lossPct},
		{"Perda técnica (kWh)", tech},
		{"Perda não técnica (kWh)", pnt},
		{"Balanços NORMAL", status[domain.BalanceStatusNormal]},
		{"Balanços ATENÇÃO", status[domain.BalanceStatusWarning]},
		{"Balanços CRÍTICO", status[domain.BalanceStatusCritical]},
		{"Metodologia", "PRODIST Módulo 7 (ANEEL), Seções 6.1, 6.2 e 6.4"},
		{"Emitido em", time.Now().Format("02/01/2006 15:04")},
	}
	for i, it := range items {
		if err := setRow(f, summary, i+2, it); err != nil {
			return nil, err
		}
	}
	_ = f.SetColWidth(summary, "A", "A", 34)
	_ = f.SetColWidth(summary, "B", "B", 40)

	// Por transformador
	detail := "Por transformador"
	if _, err := newSheet(f, detail, []string{"Mês", "Transformador", "Subestação", "Potência (kVA)", "Energia injetada (kWh)",
		"Energia faturada (kWh)", "Perda (kWh)", "Perda (%)", "Perda técnica (kWh)", "Perda não técnica (kWh)",
		"Limite ANEEL (%)", "Status", "UCs"}); err != nil {
		return nil, err
	}
	for i, r := range rows {
		sub := ""
		if r.SubstationCode != nil {
			sub = *r.SubstationCode
		}
		if err := setRow(f, detail, i+2, []any{r.PeriodStart.Format("2006-01"), r.TransformerCode, sub, r.PowerKVA,
			r.EnergyInjectedKWh, r.TotalConsumptionKWh, r.LossKWh, r.LossPct, optNum(r.TechnicalLossKWh),
			optNum(r.NonTechnicalLossKWh), optNum(r.LossLimitPct), statusPT(r.Status), r.UCCount}); err != nil {
			return nil, err
		}
	}
	_ = f.AutoFilter(detail, fmt.Sprintf("A1:M%d", len(rows)+1), nil)

	// Por subestacao e por mes
	type agg struct{ inj, cons, loss, tech, pnt float64 }
	bySub, byMonth := map[string]*agg{}, map[string]*agg{}
	for _, r := range rows {
		sub := "(sem subestação)"
		if r.SubstationCode != nil {
			sub = *r.SubstationCode
		}
		for _, entry := range []struct {
			m   map[string]*agg
			key string
		}{{bySub, sub}, {byMonth, r.PeriodStart.Format("2006-01")}} {
			a := entry.m[entry.key]
			if a == nil {
				a = &agg{}
				entry.m[entry.key] = a
			}
			a.inj += r.EnergyInjectedKWh
			a.cons += r.TotalConsumptionKWh
			a.loss += r.LossKWh
			if r.TechnicalLossKWh != nil {
				a.tech += *r.TechnicalLossKWh
			}
			if r.NonTechnicalLossKWh != nil {
				a.pnt += *r.NonTechnicalLossKWh
			}
		}
	}
	writeAgg := func(sheet, keyTitle string, m map[string]*agg) error {
		if _, err := newSheet(f, sheet, []string{keyTitle, "Energia injetada (kWh)", "Energia faturada (kWh)", "Perda (kWh)",
			"Perda (%)", "Perda técnica (kWh)", "Perda não técnica (kWh)"}); err != nil {
			return err
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			a := m[k]
			pct := 0.0
			if a.inj > 0 {
				pct = a.loss / a.inj * 100
			}
			if err := setRow(f, sheet, i+2, []any{k, a.inj, a.cons, a.loss, pct, a.tech, a.pnt}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := writeAgg("Por subestação", "Subestação", bySub); err != nil {
		return nil, err
	}
	if err := writeAgg("Mensal", "Mês", byMonth); err != nil {
		return nil, err
	}

	_ = f.DeleteSheet("Sheet1")
	f.SetActiveSheet(0)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExportGeneration, err)
	}
	name := fmt.Sprintf("relatorio_aneel_%s_%s.xlsx", start.Format("200601"), end.Format("200601"))
	return &ExportFile{Content: buf.Bytes(), ContentType: mimeXLSX, Filename: name}, nil
}

// ExportReadings exporta a serie historica de leituras em CSV (RF06.3).
// kind: transformer ou uc; transformerID opcional filtra um trafo.
func (uc *ExportUseCase) ExportReadings(ctx context.Context, tenantID domain.UUID, kind string, transformerID domain.UUID, start, end time.Time) (*ExportFile, error) {
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	w.Comma = ';'

	ids := []domain.UUID{transformerID}
	if transformerID == "" {
		all, err := uc.transformerRepo.GetByTenant(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		ids = ids[:0]
		for _, t := range all {
			ids = append(ids, t.ID)
		}
	}
	codes := map[domain.UUID]string{}
	if refs, err := uc.transformerRepo.RefsByTenant(ctx, tenantID); err == nil {
		for _, r := range refs {
			codes[r.ID] = r.Code
		}
	}

	if kind == "uc" {
		_ = w.Write([]string{"transformador", "uc", "leitura_em", "consumo_kwh"})
		for _, id := range ids {
			readings, err := uc.ucReadings.ListWithCodes(ctx, id, start, end)
			if err != nil {
				return nil, err
			}
			for _, r := range readings {
				_ = w.Write([]string{codes[r.TransformerID], r.UCCode, r.ReadingAt.Format("2006-01-02 15:04:05"), formatBR(r.ConsumptionKWh, 4)})
			}
		}
	} else {
		_ = w.Write([]string{"transformador", "leitura_em", "energia_kwh", "demanda_kw", "fator_potencia"})
		for _, id := range ids {
			readings, err := uc.trafoReadings.GetByTransformerAndPeriod(ctx, id, start, end)
			if err != nil {
				return nil, err
			}
			for _, r := range readings {
				_ = w.Write([]string{codes[r.TransformerID], r.ReadingAt.Format("2006-01-02 15:04:05"),
					formatBR(r.EnergyKWh, 4), optCSV(r.DemandKW), optCSV(r.PowerFactor)})
			}
		}
	}
	w.Flush()
	name := fmt.Sprintf("leituras_%s_%s_%s.csv", kind, start.Format("20060102"), end.Format("20060102"))
	return &ExportFile{Content: buf.Bytes(), ContentType: mimeCSV, Filename: name}, nil
}

// ExportComparative exports the month-over-month comparative report.
func (uc *ExportUseCase) ExportComparative(ctx context.Context, tenantID domain.UUID, month time.Time) (*ExportFile, error) {
	rep, err := uc.reports.Comparative(ctx, tenantID, month)
	if err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Comparativo"
	if _, err := newSheet(f, sheet, []string{"Transformador", "Subestação", "Perda " + rep.Month + " (%)",
		"Perda " + rep.Previous.Month + " (%)", "Perda " + rep.LastYear.Month + " (%)",
		"Variação mês (p.p.)", "Variação ano (p.p.)", "Perda " + rep.Month + " (kWh)", "Status"}); err != nil {
		return nil, err
	}
	for i, r := range rep.Rows {
		sub := ""
		if r.SubstationCode != nil {
			sub = *r.SubstationCode
		}
		if err := setRow(f, sheet, i+2, []any{r.TransformerCode, sub, optNum(r.CurrentPct), optNum(r.PreviousPct),
			optNum(r.LastYearPct), optNum(r.DeltaMonthPP), optNum(r.DeltaYearPP), optNum(r.CurrentLossKWh), r.Status}); err != nil {
			return nil, err
		}
	}
	_ = f.DeleteSheet("Sheet1")
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExportGeneration, err)
	}
	return &ExportFile{Content: buf.Bytes(), ContentType: mimeXLSX, Filename: "comparativo_" + rep.Month + ".xlsx"}, nil
}
