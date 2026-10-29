package usecase

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// ReportUseCase builds analytical views over balances and readings.
type ReportUseCase struct {
	balanceRepo     *repository.BalanceRepository
	transformerRepo *repository.TransformerRepository
	trafoReadings   *repository.TransformerReadingRepository
	ucReadings      *repository.UCReadingRepository
	transformers    *TransformerUseCase
}

// NewReportUseCase creates a ReportUseCase
func NewReportUseCase(
	balanceRepo *repository.BalanceRepository,
	transformerRepo *repository.TransformerRepository,
	trafoReadings *repository.TransformerReadingRepository,
	ucReadings *repository.UCReadingRepository,
	transformers *TransformerUseCase,
) *ReportUseCase {
	return &ReportUseCase{
		balanceRepo:     balanceRepo,
		transformerRepo: transformerRepo,
		trafoReadings:   trafoReadings,
		ucReadings:      ucReadings,
		transformers:    transformers,
	}
}

// MonthlyLoss is one month of a transformer loss analysis.
type MonthlyLoss struct {
	Month               string   `json:"month"`
	EnergyInjectedKWh   float64  `json:"energy_injected_kwh"`
	TotalConsumptionKWh float64  `json:"total_consumption_kwh"`
	LossKWh             float64  `json:"loss_kwh"`
	LossPct             float64  `json:"loss_pct"`
	TechnicalLossKWh    *float64 `json:"technical_loss_kwh"`
	NonTechnicalLossKWh *float64 `json:"non_technical_loss_kwh"`
	Status              string   `json:"status"`
	LimitPct            float64  `json:"limit_pct"`
	UCCount             int      `json:"uc_count"`
}

// LossAnalysis is the loss breakdown of a transformer over several months.
type LossAnalysis struct {
	Transformer     *domain.Transformer `json:"transformer"`
	Months          []MonthlyLoss       `json:"months"`
	Stats           *TransformerStats   `json:"stats"`
	NonTechnicalPct *float64            `json:"non_technical_share_pct"` // PNT / perda total
	Insights        []string            `json:"insights"`
}

// TransformerLossAnalysis returns the monthly balances of a transformer and
// insights for the engineering team.
func (uc *ReportUseCase) TransformerLossAnalysis(ctx context.Context, id domain.UUID, months int, _ domain.UserRole) (*LossAnalysis, error) {
	t, err := uc.transformerRepo.GetByID(ctx, id)
	if err != nil || t == nil {
		return nil, ErrTransformerNotFound
	}
	if months <= 0 || months > 60 {
		months = 12
	}
	end := time.Now()
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location()).AddDate(0, -(months - 1), 0)
	balances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, id, start, end)
	if err != nil {
		return nil, fmt.Errorf("load balances: %w", err)
	}

	res := &LossAnalysis{Transformer: t, Months: []MonthlyLoss{}, Insights: []string{}}
	limit := t.LossLimitPctValue()
	var totalLoss, totalPNT float64
	hasPNT := false
	for i := len(balances) - 1; i >= 0; i-- { // mais antigo primeiro
		b := balances[i]
		res.Months = append(res.Months, MonthlyLoss{
			Month: b.PeriodStart.Format("2006-01"), EnergyInjectedKWh: b.EnergyInjectedKWh,
			TotalConsumptionKWh: b.TotalConsumptionKWh, LossKWh: b.LossKWh, LossPct: b.LossPct,
			TechnicalLossKWh: b.TechnicalLossKWh, NonTechnicalLossKWh: b.NonTechnicalLossKWh,
			Status: string(b.Status), LimitPct: limit, UCCount: b.UCCount,
		})
		totalLoss += b.LossKWh
		if b.NonTechnicalLossKWh != nil {
			totalPNT += *b.NonTechnicalLossKWh
			hasPNT = true
		}
	}
	if stats, err := uc.transformers.GetTransformerStats(ctx, id); err == nil {
		res.Stats = stats
	}
	if hasPNT && totalLoss > 0 {
		share := round(totalPNT/totalLoss*100, 1)
		res.NonTechnicalPct = &share
	}
	res.Insights = lossInsights(t, res)
	return res, nil
}

// lossInsights gera recomendacoes simples a partir do historico.
func lossInsights(t *domain.Transformer, a *LossAnalysis) []string {
	var out []string
	if t.CoreLossKW == nil || t.WindingLossKW == nil {
		out = append(out, "Cadastre P0 e Pcc (dados de placa) para separar perdas técnicas e não técnicas conforme PRODIST Módulo 7.")
	}
	if len(a.Months) == 0 {
		return append(out, "Sem balanços calculados no período; importe leituras e calcule o balanço.")
	}
	critical := 0
	for _, m := range a.Months {
		if m.Status == string(domain.BalanceStatusCritical) {
			critical++
		}
	}
	if critical >= 3 {
		out = append(out, fmt.Sprintf("Perda acima do limite regulatório em %d meses; priorize inspeção de campo.", critical))
	}
	if a.NonTechnicalPct != nil && *a.NonTechnicalPct > 50 {
		out = append(out, fmt.Sprintf("Perdas não técnicas representam %.0f%% da perda total: há indício de fraude, furto ou erro de medição.", *a.NonTechnicalPct))
	}
	if a.Stats != nil && a.Stats.Trend == "UP" {
		out = append(out, "A perda subiu em relação ao mês anterior.")
	}
	if len(out) == 0 {
		out = append(out, "Perdas dentro do esperado no período.")
	}
	return out
}

// SeriesResult is the reading series of a transformer.
type SeriesResult struct {
	TransformerID domain.UUID              `json:"transformer_id"`
	Bucket        string                   `json:"bucket"`
	Points        []repository.SeriesPoint `json:"points"`
}

// TransformerSeries returns the energy series of a transformer.
func (uc *ReportUseCase) TransformerSeries(ctx context.Context, id domain.UUID, start, end time.Time, bucket string) (*SeriesResult, error) {
	t, err := uc.transformerRepo.GetByID(ctx, id)
	if err != nil || t == nil {
		return nil, ErrTransformerNotFound
	}
	if bucket != "day" && bucket != "week" {
		bucket = "month"
	}
	points, err := uc.trafoReadings.Series(ctx, id, start, end, bucket)
	if err != nil {
		return nil, fmt.Errorf("load series: %w", err)
	}
	if points == nil {
		points = []repository.SeriesPoint{}
	}
	return &SeriesResult{TransformerID: id, Bucket: bucket, Points: points}, nil
}

// TopConsumersResult lists the UCs of a transformer by consumption.
type TopConsumersResult struct {
	TransformerID domain.UUID                `json:"transformer_id"`
	PeriodStart   time.Time                  `json:"period_start"`
	PeriodEnd     time.Time                  `json:"period_end"`
	TotalKWh      float64                    `json:"total_kwh"`
	Consumers     []repository.UCConsumption `json:"consumers"`
	WithoutRead   int                        `json:"ucs_without_reading"`
}

// TopConsumers returns the UCs of a transformer ordered by consumption.
func (uc *ReportUseCase) TopConsumers(ctx context.Context, id domain.UUID, start, end time.Time, limit int) (*TopConsumersResult, error) {
	t, err := uc.transformerRepo.GetByID(ctx, id)
	if err != nil || t == nil {
		return nil, ErrTransformerNotFound
	}
	start, end, err = NormalizePeriod(start, end)
	if err != nil {
		return nil, err
	}
	all, err := uc.ucReadings.ConsumptionByUC(ctx, id, start, end, 0)
	if err != nil {
		return nil, fmt.Errorf("load consumption: %w", err)
	}
	res := &TopConsumersResult{TransformerID: id, PeriodStart: start, PeriodEnd: end, Consumers: []repository.UCConsumption{}}
	for _, c := range all {
		res.TotalKWh += c.ConsumptionKWh
		if c.Readings == 0 {
			res.WithoutRead++
		}
	}
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	res.Consumers = append(res.Consumers, all[:limit]...)
	return res, nil
}

// ComparativeRow compares a transformer's loss across months.
type ComparativeRow struct {
	TransformerID   domain.UUID `json:"transformer_id"`
	TransformerCode string      `json:"transformer_code"`
	SubstationCode  *string     `json:"substation_code"`
	CurrentPct      *float64    `json:"current_loss_pct"`
	PreviousPct     *float64    `json:"previous_month_loss_pct"`
	LastYearPct     *float64    `json:"same_month_last_year_loss_pct"`
	DeltaMonthPP    *float64    `json:"delta_month_pp"`
	DeltaYearPP     *float64    `json:"delta_year_pp"`
	CurrentLossKWh  *float64    `json:"current_loss_kwh"`
	Status          string      `json:"status"`
}

// ComparativeTotals compares the tenant totals.
type ComparativeTotals struct {
	Month       string   `json:"month"`
	LossPct     *float64 `json:"loss_pct"`
	LossKWh     float64  `json:"loss_kwh"`
	InjectedKWh float64  `json:"energy_injected_kwh"`
	Count       int      `json:"transformers"`
}

// ComparativeReport is the month-over-month report (Sprint 3).
type ComparativeReport struct {
	Month    string              `json:"month"`
	Current  ComparativeTotals   `json:"current"`
	Previous ComparativeTotals   `json:"previous"`
	LastYear ComparativeTotals   `json:"last_year"`
	Rows     []ComparativeRow    `json:"rows"`
	Improved int                 `json:"improved"`
	Worsened int                 `json:"worsened"`
	Biggest  []ComparativeRow    `json:"biggest_increases"`
	Periods  map[string][]string `json:"periods"`
}

// Comparative compares each transformer's loss in month with the previous
// month and with the same month of the previous year.
func (uc *ReportUseCase) Comparative(ctx context.Context, tenantID domain.UUID, month time.Time) (*ComparativeReport, error) {
	cur := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
	prev := cur.AddDate(0, -1, 0)
	ly := cur.AddDate(-1, 0, 0)

	rows, err := uc.balanceRepo.ListRowsByTenantPeriod(ctx, tenantID, ly, cur.AddDate(0, 1, 0).Add(-time.Nanosecond))
	if err != nil {
		return nil, fmt.Errorf("load balances: %w", err)
	}

	type key struct {
		t domain.UUID
		m string
	}
	byKey := map[key]*repository.BalanceRow{}
	codes := map[domain.UUID]*repository.BalanceRow{}
	for _, r := range rows {
		m := r.PeriodStart.Format("2006-01")
		byKey[key{r.TransformerID, m}] = r
		codes[r.TransformerID] = r
	}

	curM, prevM, lyM := cur.Format("2006-01"), prev.Format("2006-01"), ly.Format("2006-01")
	rep := &ComparativeReport{
		Month:    curM,
		Current:  ComparativeTotals{Month: curM},
		Previous: ComparativeTotals{Month: prevM},
		LastYear: ComparativeTotals{Month: lyM},
		Rows:     []ComparativeRow{},
		Periods:  map[string][]string{"current": {curM}, "previous": {prevM}, "last_year": {lyM}},
	}
	addTotals := func(t *ComparativeTotals, r *repository.BalanceRow) {
		t.LossKWh += r.LossKWh
		t.InjectedKWh += r.EnergyInjectedKWh
		t.Count++
	}

	for id, ref := range codes {
		c, p, l := byKey[key{id, curM}], byKey[key{id, prevM}], byKey[key{id, lyM}]
		if c == nil && p == nil {
			continue
		}
		row := ComparativeRow{TransformerID: id, TransformerCode: ref.TransformerCode, SubstationCode: ref.SubstationCode, Status: "NO_DATA"}
		if c != nil {
			v, lk := c.LossPct, c.LossKWh
			row.CurrentPct, row.CurrentLossKWh, row.Status = &v, &lk, string(c.Status)
			addTotals(&rep.Current, c)
		}
		if p != nil {
			v := p.LossPct
			row.PreviousPct = &v
			addTotals(&rep.Previous, p)
		}
		if l != nil {
			v := l.LossPct
			row.LastYearPct = &v
			addTotals(&rep.LastYear, l)
		}
		if row.CurrentPct != nil && row.PreviousPct != nil {
			d := round(*row.CurrentPct-*row.PreviousPct, 2)
			row.DeltaMonthPP = &d
			if d < -0.1 {
				rep.Improved++
			} else if d > 0.1 {
				rep.Worsened++
			}
		}
		if row.CurrentPct != nil && row.LastYearPct != nil {
			d := round(*row.CurrentPct-*row.LastYearPct, 2)
			row.DeltaYearPP = &d
		}
		rep.Rows = append(rep.Rows, row)
	}
	for _, t := range []*ComparativeTotals{&rep.Current, &rep.Previous, &rep.LastYear} {
		if t.InjectedKWh > 0 {
			v := round(t.LossKWh/t.InjectedKWh*100, 2)
			t.LossPct = &v
		}
	}
	sort.Slice(rep.Rows, func(i, j int) bool { return rep.Rows[i].TransformerCode < rep.Rows[j].TransformerCode })

	ranked := append([]ComparativeRow{}, rep.Rows...)
	sort.Slice(ranked, func(i, j int) bool {
		return deltaOrMin(ranked[i].DeltaMonthPP) > deltaOrMin(ranked[j].DeltaMonthPP)
	})
	for _, r := range ranked {
		if r.DeltaMonthPP == nil || *r.DeltaMonthPP <= 0 || len(rep.Biggest) == 10 {
			break
		}
		rep.Biggest = append(rep.Biggest, r)
	}
	if rep.Biggest == nil {
		rep.Biggest = []ComparativeRow{}
	}
	return rep, nil
}

func deltaOrMin(v *float64) float64 {
	if v == nil {
		return -1e18
	}
	return *v
}
