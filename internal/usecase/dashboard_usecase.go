package usecase

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// DefaultTariffBRLMWh is used when the tenant has not configured a tariff.
const DefaultTariffBRLMWh = 450.0

// staleReadingDays marks a transformer without readings for this long.
const staleReadingDays = 35

// DashboardUseCase builds the map, operational and managerial dashboards.
type DashboardUseCase struct {
	transformerRepo *repository.TransformerRepository
	balanceRepo     *repository.BalanceRepository
	substationRepo  *repository.SubstationRepository
	accountRepo     *repository.AccountRepository
	alertRepo       *repository.AlertRepository
	importRepo      *repository.ImportRepository
}

// NewDashboardUseCase creates a DashboardUseCase
func NewDashboardUseCase(
	transformerRepo *repository.TransformerRepository,
	balanceRepo *repository.BalanceRepository,
	substationRepo *repository.SubstationRepository,
	accountRepo *repository.AccountRepository,
	alertRepo *repository.AlertRepository,
	importRepo *repository.ImportRepository,
) *DashboardUseCase {
	return &DashboardUseCase{
		transformerRepo: transformerRepo, balanceRepo: balanceRepo, substationRepo: substationRepo,
		accountRepo: accountRepo, alertRepo: alertRepo, importRepo: importRepo,
	}
}

// Tariff resolves the tariff (R$/MWh): explicit override, tenant setting or
// the default. The second value says where it came from.
func (uc *DashboardUseCase) Tariff(ctx context.Context, tenantID domain.UUID, override float64) (float64, string) {
	if override > 0 && override <= 10000 {
		return override, "query"
	}
	if uc.accountRepo != nil {
		if s, err := uc.accountRepo.GetSettings(ctx, tenantID); err == nil && s != nil && s.TariffBRLMWh != nil && *s.TariffBRLMWh > 0 {
			return *s.TariffBRLMWh, "tenant"
		}
	}
	return DefaultTariffBRLMWh, "default"
}

func limitOf(t *domain.Transformer) float64 {
	if t.LossLimitPct != nil && *t.LossLimitPct > 0 {
		return *t.LossLimitPct
	}
	return 10
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func pct(part, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return round2(part / total * 100)
}

// Summaries returns every transformer of the tenant matching f with its
// latest balance (uma unica consulta agregada).
func (uc *DashboardUseCase) Summaries(ctx context.Context, tenantID domain.UUID, f repository.TransformerFilter) ([]*repository.TransformerSummary, error) {
	return uc.allSummaries(ctx, tenantID, f)
}

// Evolution returns the monthly evolution of the last months with the
// average regulatory limit of the network, without loading the transformers.
func (uc *DashboardUseCase) Evolution(ctx context.Context, tenantID domain.UUID, months int) ([]MonthPoint, error) {
	avgLimit, err := uc.transformerRepo.AvgLossLimit(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return uc.evolution(ctx, tenantID, months, avgLimit)
}

// allSummaries returns every transformer of the tenant with its latest balance.
func (uc *DashboardUseCase) allSummaries(ctx context.Context, tenantID domain.UUID, f repository.TransformerFilter) ([]*repository.TransformerSummary, error) {
	f.Limit = 0
	items, _, err := uc.transformerRepo.ListSummaries(ctx, tenantID, f)
	return items, err
}

// MapPoint is one transformer on the map with what its popup shows.
type MapPoint struct {
	ID                  domain.UUID `json:"id"`
	Code                string      `json:"code"`
	Lat                 float64     `json:"lat"`
	Lng                 float64     `json:"lng"`
	Status              string      `json:"status"`
	Active              bool        `json:"active"`
	PowerKVA            float64     `json:"power_kva"`
	SubstationID        *string     `json:"substation_id"`
	SubstationCode      *string     `json:"substation_code"`
	SubstationName      *string     `json:"substation_name"`
	UCCount             int         `json:"uc_count"`
	LossPct             *float64    `json:"loss_pct"`
	LossLimitPct        float64     `json:"loss_limit_pct"`
	LimitUsagePct       *float64    `json:"limit_usage_pct"`
	LossKWh             *float64    `json:"loss_kwh"`
	EnergyInjectedKWh   *float64    `json:"energy_injected_kwh"`
	TotalConsumptionKWh *float64    `json:"total_consumption_kwh"`
	TechnicalLossKWh    *float64    `json:"technical_loss_kwh"`
	NonTechnicalLossKWh *float64    `json:"non_technical_loss_kwh"`
	LoadFactorPct       *float64    `json:"load_factor_pct"`
	LastPeriodStart     *time.Time  `json:"last_period_start"`
	LastPeriodEnd       *time.Time  `json:"last_period_end"`
	LastReadingAt       *time.Time  `json:"last_reading_at"`
	StaleReadings       bool        `json:"stale_readings"`
	AlertsLast30d       int         `json:"alerts_last_30d"`
	LossCostBRL         *float64    `json:"loss_cost_brl,omitempty"`
	RecoverableBRL      *float64    `json:"recoverable_brl,omitempty"`
}

// MapSubstation is a substation marker.
type MapSubstation struct {
	ID               domain.UUID `json:"id"`
	Code             string      `json:"code"`
	Name             string      `json:"name"`
	Lat              *float64    `json:"lat"`
	Lng              *float64    `json:"lng"`
	TransformerCount int         `json:"transformer_count"`
	TotalKVA         float64     `json:"total_kva"`
	CriticalCount    int         `json:"critical_count"`
	WarningCount     int         `json:"warning_count"`
}

// MapBounds is the bounding box of the points.
type MapBounds struct {
	MinLat float64 `json:"min_lat"`
	MinLng float64 `json:"min_lng"`
	MaxLat float64 `json:"max_lat"`
	MaxLng float64 `json:"max_lng"`
}

// MapData is the map payload (RF07 mapa georreferenciado).
type MapData struct {
	Transformers    []MapPoint      `json:"transformers"`
	Substations     []MapSubstation `json:"substations"`
	Bounds          *MapBounds      `json:"bounds"`
	Center          *[2]float64     `json:"center"`
	ByStatus        map[string]int  `json:"by_status"`
	Total           int             `json:"total"`
	WithoutLocation int             `json:"without_location"`
	MissingCodes    []string        `json:"missing_location_codes"`
	TariffBRLMWh    *float64        `json:"tariff_brl_mwh,omitempty"`
	// Located e o total de trafos com coordenadas que atendem ao filtro;
	// InView os que estao na area pedida; Truncated indica que nem todos os
	// pontos da area vieram (priorizados por criticidade).
	Located   int  `json:"located"`
	InView    int  `json:"in_view"`
	Truncated bool `json:"truncated"`
}

// MapFilter filters the map points. BBox (min_lat, min_lng, max_lat, max_lng)
// restringe os pontos devolvidos a area visivel; Limit limita a quantidade
// (os contadores e bounds continuam valendo para a rede inteira).
type MapFilter struct {
	Status       string
	SubstationID string
	GroupID      string
	Search       string
	BBox         *MapBounds
	Limit        int
}

// Limites de pontos por resposta do mapa: com dezenas de milhares de trafos o
// payload completo passava de 30 MB.
const (
	DefaultMapPoints = 3000
	MaxMapPoints     = 10000
)

// Map returns the georeferenced transformers with their latest balance.
// Valores em R$ so sao preenchidos quando financial for true.
func (uc *DashboardUseCase) Map(ctx context.Context, tenantID domain.UUID, f MapFilter, financial bool) (*MapData, error) {
	items, err := uc.allSummaries(ctx, tenantID, repository.TransformerFilter{
		Status: f.Status, SubstationID: f.SubstationID, GroupID: f.GroupID, Search: f.Search,
	})
	if err != nil {
		return nil, err
	}
	out := &MapData{Transformers: []MapPoint{}, Substations: []MapSubstation{}, ByStatus: map[string]int{
		"NORMAL": 0, "WARNING": 0, "CRITICAL": 0, "NO_DATA": 0,
	}, MissingCodes: []string{}}
	var tariff float64
	if financial {
		tariff, _ = uc.Tariff(ctx, tenantID, 0)
		out.TariffBRLMWh = &tariff
	}
	staleBefore := time.Now().AddDate(0, 0, -staleReadingDays)
	var b *MapBounds
	var sumLat, sumLng float64
	for _, s := range items {
		out.Total++
		out.ByStatus[s.Status]++
		if s.Lat == nil || s.Lng == nil || (*s.Lat == 0 && *s.Lng == 0) {
			out.WithoutLocation++
			if len(out.MissingCodes) < 50 {
				out.MissingCodes = append(out.MissingCodes, s.Code)
			}
			continue
		}
		limit := limitOf(&s.Transformer)
		p := MapPoint{
			ID: s.ID, Code: s.Code, Lat: *s.Lat, Lng: *s.Lng, Status: s.Status, Active: s.Active,
			PowerKVA: s.PowerKVA, SubstationCode: s.SubstationCode, SubstationName: s.SubstationName,
			UCCount: s.UCCount, LossPct: s.LossPct, LossLimitPct: limit, LossKWh: s.LossKWh,
			EnergyInjectedKWh: s.EnergyInjectedKWh, TotalConsumptionKWh: s.TotalConsumptionKWh,
			TechnicalLossKWh: s.TechnicalLossKWh, NonTechnicalLossKWh: s.NonTechnicalLossKWh,
			LastPeriodStart: s.LastPeriodStart, LastPeriodEnd: s.LastPeriodEnd, LastReadingAt: s.LastReadingAt,
			StaleReadings: s.LastReadingAt == nil || s.LastReadingAt.Before(staleBefore),
			AlertsLast30d: s.AlertsLast30d,
		}
		if s.SubstationID != nil {
			id := string(*s.SubstationID)
			p.SubstationID = &id
		}
		if s.LossPct != nil {
			v := round2(*s.LossPct / limit * 100)
			p.LimitUsagePct = &v
		}
		// Fator de carga: energia injetada no periodo sobre a capacidade
		// nominal (kVA x horas, fp 1) - indicador de sobrecarga para o mapa.
		if s.EnergyInjectedKWh != nil && s.LastPeriodStart != nil && s.LastPeriodEnd != nil && s.PowerKVA > 0 {
			hours := s.LastPeriodEnd.Sub(*s.LastPeriodStart).Hours()
			if hours > 0 {
				v := round2(*s.EnergyInjectedKWh / (s.PowerKVA * hours) * 100)
				p.LoadFactorPct = &v
			}
		}
		if financial && s.LossKWh != nil {
			cost := round2(*s.LossKWh / 1000 * tariff)
			excess := math.Max(0, *s.LossKWh-deref(s.EnergyInjectedKWh)*limit/100)
			rec := round2(excess / 1000 * tariff)
			p.LossCostBRL, p.RecoverableBRL = &cost, &rec
		}
		out.Located++
		sumLat += p.Lat
		sumLng += p.Lng
		if b == nil {
			b = &MapBounds{MinLat: p.Lat, MaxLat: p.Lat, MinLng: p.Lng, MaxLng: p.Lng}
		} else {
			b.MinLat, b.MaxLat = math.Min(b.MinLat, p.Lat), math.Max(b.MaxLat, p.Lat)
			b.MinLng, b.MaxLng = math.Min(b.MinLng, p.Lng), math.Max(b.MaxLng, p.Lng)
		}
		if f.BBox != nil && (p.Lat < f.BBox.MinLat || p.Lat > f.BBox.MaxLat || p.Lng < f.BBox.MinLng || p.Lng > f.BBox.MaxLng) {
			continue
		}
		out.Transformers = append(out.Transformers, p)
	}
	if out.Located > 0 {
		n := float64(out.Located)
		out.Bounds = b
		out.Center = &[2]float64{sumLat / n, sumLng / n}
	}
	out.InView = len(out.Transformers)
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultMapPoints
	}
	limit = min(limit, MaxMapPoints)
	if len(out.Transformers) > limit {
		// Prioriza o que pede acao: criticos, alerta, leituras atrasadas.
		rank := map[string]int{"CRITICAL": 0, "WARNING": 1, "NORMAL": 2, "NO_DATA": 3}
		sort.SliceStable(out.Transformers, func(i, j int) bool {
			a, c := out.Transformers[i], out.Transformers[j]
			if rank[a.Status] != rank[c.Status] {
				return rank[a.Status] < rank[c.Status]
			}
			if a.StaleReadings != c.StaleReadings {
				return a.StaleReadings
			}
			return deref(a.LimitUsagePct) > deref(c.LimitUsagePct)
		})
		out.Transformers = out.Transformers[:limit]
		out.Truncated = true
	}
	if uc.substationRepo != nil {
		subs, err := uc.substationRepo.List(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		for _, s := range subs {
			out.Substations = append(out.Substations, MapSubstation{
				ID: s.ID, Code: s.Code, Name: s.Name, Lat: s.Lat, Lng: s.Lng,
				TransformerCount: s.TransformerCount, TotalKVA: s.TotalKVA,
				CriticalCount: s.CriticalCount, WarningCount: s.WarningCount,
			})
		}
	}
	return out, nil
}

// MonthPoint is one month of the operational evolution.
type MonthPoint struct {
	Month           string  `json:"month"`
	InjectedMWh     float64 `json:"injected_mwh"`
	BilledMWh       float64 `json:"billed_mwh"`
	LossMWh         float64 `json:"loss_mwh"`
	LossPct         float64 `json:"loss_pct"`
	TechnicalPct    float64 `json:"technical_pct"`
	NonTechnicalPct float64 `json:"non_technical_pct"`
	LimitPct        float64 `json:"limit_pct"`
	Balances        int     `json:"balances"`
}

// AttentionItem is a transformer that needs action, with the reason.
type AttentionItem struct {
	ID             domain.UUID `json:"id"`
	Code           string      `json:"code"`
	SubstationName *string     `json:"substation_name"`
	Status         string      `json:"status"`
	LossPct        *float64    `json:"loss_pct"`
	LossLimitPct   float64     `json:"loss_limit_pct"`
	ExcessPP       float64     `json:"excess_pp"`
	LastReadingAt  *time.Time  `json:"last_reading_at"`
	AlertsLast30d  int         `json:"alerts_last_30d"`
	Reasons        []string    `json:"reasons"`
}

// OperationalDashboard is the engineering view: % and kWh, no R$.
type OperationalDashboard struct {
	GeneratedAt time.Time `json:"generated_at"`
	Network     struct {
		Transformers   int     `json:"transformers"`
		Active         int     `json:"active"`
		WithLocation   int     `json:"with_location"`
		Substations    int     `json:"substations"`
		ConsumingUnits int     `json:"consuming_units"`
		TotalKVA       float64 `json:"total_kva"`
	} `json:"network"`
	Status      map[string]int `json:"status"`
	LatestMonth *MonthPoint    `json:"latest_month"`
	PrevMonth   *MonthPoint    `json:"previous_month"`
	LossDeltaPP *float64       `json:"loss_delta_pp"`
	AboveLimit  int            `json:"above_limit"`
	DataQuality struct {
		StaleReadings   int        `json:"stale_readings"`
		NeverRead       int        `json:"never_read"`
		NoBalance       int        `json:"no_balance"`
		CoveragePct     float64    `json:"coverage_pct"`
		LastReadingAt   *time.Time `json:"last_reading_at"`
		WithoutLocation int        `json:"without_location"`
	} `json:"data_quality"`
	Imports     *repository.ImportSummary       `json:"imports"`
	Alerts      *repository.AlertStats          `json:"alerts_30d"`
	Attention   []AttentionItem                 `json:"attention"`
	Evolution   []MonthPoint                    `json:"evolution"`
	Substations []*repository.SubstationSummary `json:"substations"`
}

func (uc *DashboardUseCase) evolution(ctx context.Context, tenantID domain.UUID, months int, avgLimit float64) ([]MonthPoint, error) {
	aggs, err := uc.balanceRepo.GetMonthlyAggregatesByTenant(ctx, tenantID, months)
	if err != nil {
		return nil, err
	}
	out := make([]MonthPoint, 0, len(aggs))
	for _, a := range aggs {
		out = append(out, MonthPoint{
			Month:           a.Month.Format("2006-01"),
			InjectedMWh:     round2(a.TotalEnergyInjected / 1000),
			BilledMWh:       round2(a.TotalConsumption / 1000),
			LossMWh:         round2(a.TotalLoss / 1000),
			LossPct:         pct(a.TotalLoss, a.TotalEnergyInjected),
			TechnicalPct:    pct(a.TotalTechnicalLoss, a.TotalEnergyInjected),
			NonTechnicalPct: pct(a.TotalNonTechnicalLoss, a.TotalEnergyInjected),
			LimitPct:        round2(avgLimit),
			Balances:        a.BalanceCount,
		})
	}
	return out, nil
}

// Operational builds the operational dashboard.
func (uc *DashboardUseCase) Operational(ctx context.Context, tenantID domain.UUID) (*OperationalDashboard, error) {
	items, err := uc.allSummaries(ctx, tenantID, repository.TransformerFilter{})
	if err != nil {
		return nil, err
	}
	d := &OperationalDashboard{GeneratedAt: time.Now(), Status: map[string]int{"NORMAL": 0, "WARNING": 0, "CRITICAL": 0, "NO_DATA": 0},
		Attention: []AttentionItem{}}
	staleBefore := time.Now().AddDate(0, 0, -staleReadingDays)
	var sumLimit float64
	for _, s := range items {
		d.Network.Transformers++
		d.Network.ConsumingUnits += s.UCCount
		d.Network.TotalKVA += s.PowerKVA
		if s.Active {
			d.Network.Active++
		}
		if s.Lat != nil && s.Lng != nil {
			d.Network.WithLocation++
		} else {
			d.DataQuality.WithoutLocation++
		}
		d.Status[s.Status]++
		limit := limitOf(&s.Transformer)
		sumLimit += limit
		var reasons []string
		if s.LastReadingAt == nil {
			d.DataQuality.NeverRead++
		} else {
			if d.DataQuality.LastReadingAt == nil || s.LastReadingAt.After(*d.DataQuality.LastReadingAt) {
				t := *s.LastReadingAt
				d.DataQuality.LastReadingAt = &t
			}
			if s.LastReadingAt.Before(staleBefore) {
				d.DataQuality.StaleReadings++
				if s.Active {
					reasons = append(reasons, "sem leituras há mais de 35 dias")
				}
			}
		}
		if s.LossPct == nil {
			d.DataQuality.NoBalance++
		}
		excess := 0.0
		switch s.Status {
		case string(domain.BalanceStatusCritical):
			d.AboveLimit++
			excess = round2(*s.LossPct - limit)
			reasons = append(reasons, "perda acima do limite regulatório")
		case string(domain.BalanceStatusWarning):
			excess = round2(*s.LossPct - limit)
			reasons = append(reasons, "perda acima de 80% do limite")
		}
		if s.AlertsLast30d >= 3 {
			reasons = append(reasons, "alertas recorrentes nos últimos 30 dias")
		}
		if len(reasons) > 0 && s.Active {
			d.Attention = append(d.Attention, AttentionItem{
				ID: s.ID, Code: s.Code, SubstationName: s.SubstationName, Status: s.Status, LossPct: s.LossPct,
				LossLimitPct: limit, ExcessPP: excess, LastReadingAt: s.LastReadingAt,
				AlertsLast30d: s.AlertsLast30d, Reasons: reasons,
			})
		}
	}
	d.Network.TotalKVA = round2(d.Network.TotalKVA)
	if d.Network.Transformers > 0 {
		d.DataQuality.CoveragePct = pct(float64(d.Network.Transformers-d.DataQuality.NoBalance), float64(d.Network.Transformers))
	}
	rank := map[string]int{"CRITICAL": 0, "WARNING": 1, "NORMAL": 2, "NO_DATA": 3}
	sort.SliceStable(d.Attention, func(i, j int) bool {
		a, b := d.Attention[i], d.Attention[j]
		if rank[a.Status] != rank[b.Status] {
			return rank[a.Status] < rank[b.Status]
		}
		return a.ExcessPP > b.ExcessPP
	})
	if len(d.Attention) > 20 {
		d.Attention = d.Attention[:20]
	}

	avgLimit := 10.0
	if len(items) > 0 {
		avgLimit = sumLimit / float64(len(items))
	}
	if d.Evolution, err = uc.evolution(ctx, tenantID, 12, avgLimit); err != nil {
		return nil, err
	}
	if n := len(d.Evolution); n > 0 {
		d.LatestMonth = &d.Evolution[n-1]
		if n > 1 {
			d.PrevMonth = &d.Evolution[n-2]
			delta := round2(d.LatestMonth.LossPct - d.PrevMonth.LossPct)
			d.LossDeltaPP = &delta
		}
	}
	if uc.substationRepo != nil {
		if d.Substations, err = uc.substationRepo.List(ctx, tenantID); err != nil {
			return nil, err
		}
		d.Network.Substations = len(d.Substations)
	}
	if uc.importRepo != nil {
		if d.Imports, err = uc.importRepo.Summary(ctx, tenantID); err != nil {
			return nil, err
		}
	}
	if uc.alertRepo != nil {
		if d.Alerts, err = uc.alertRepo.Stats(ctx, tenantID, time.Now().AddDate(0, 0, -30)); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// MonthFinance is the financial result of one month (or a total).
type MonthFinance struct {
	Month          string  `json:"month,omitempty"`
	InjectedMWh    float64 `json:"injected_mwh"`
	BilledMWh      float64 `json:"billed_mwh"`
	LossMWh        float64 `json:"loss_mwh"`
	LossPct        float64 `json:"loss_pct"`
	ExcessLossMWh  float64 `json:"excess_loss_mwh"`
	LossCostBRL    float64 `json:"loss_cost_brl"`
	RevenueBRL     float64 `json:"revenue_brl"`
	RecoverableBRL float64 `json:"recoverable_brl"`
	Transformers   int     `json:"transformers"`
}

func (m *MonthFinance) add(row *repository.BalanceRow, tariff float64) {
	limit := deref(row.LossLimitPct)
	if limit <= 0 {
		limit = 10
	}
	excess := math.Max(0, row.LossKWh-row.EnergyInjectedKWh*limit/100)
	m.InjectedMWh += row.EnergyInjectedKWh / 1000
	m.BilledMWh += row.TotalConsumptionKWh / 1000
	m.LossMWh += row.LossKWh / 1000
	m.ExcessLossMWh += excess / 1000
	m.LossCostBRL += row.LossKWh / 1000 * tariff
	m.RevenueBRL += row.TotalConsumptionKWh / 1000 * tariff
	m.RecoverableBRL += excess / 1000 * tariff
	m.Transformers++
}

func (m *MonthFinance) finish() {
	m.LossPct = pct(m.LossMWh, m.InjectedMWh)
	m.InjectedMWh, m.BilledMWh, m.LossMWh = round2(m.InjectedMWh), round2(m.BilledMWh), round2(m.LossMWh)
	m.ExcessLossMWh = round2(m.ExcessLossMWh)
	m.LossCostBRL, m.RevenueBRL, m.RecoverableBRL = round2(m.LossCostBRL), round2(m.RevenueBRL), round2(m.RecoverableBRL)
}

// SubstationFinance aggregates the last month by substation.
type SubstationFinance struct {
	Substation string `json:"substation"`
	MonthFinance
}

// RecoverableItem is a transformer ranked by recoverable value.
type RecoverableItem struct {
	TransformerID  domain.UUID `json:"transformer_id"`
	Code           string      `json:"code"`
	Substation     *string     `json:"substation"`
	LossPct        float64     `json:"loss_pct"`
	LossLimitPct   float64     `json:"loss_limit_pct"`
	ExcessLossKWh  float64     `json:"excess_loss_kwh"`
	LossCostBRL    float64     `json:"loss_cost_brl"`
	RecoverableBRL float64     `json:"recoverable_brl"`
	Status         string      `json:"status"`
}

// ManagerialDashboard is the financial view (MANAGER and above).
type ManagerialDashboard struct {
	GeneratedAt       time.Time           `json:"generated_at"`
	TariffBRLMWh      float64             `json:"tariff_brl_mwh"`
	TariffSource      string              `json:"tariff_source"`
	CurrentMonth      *MonthFinance       `json:"current_month"`
	PreviousMonth     *MonthFinance       `json:"previous_month"`
	DeltaLossCostPct  *float64            `json:"delta_loss_cost_pct"`
	DeltaLossPP       *float64            `json:"delta_loss_pp"`
	YearToDate        MonthFinance        `json:"year_to_date"`
	Last12Months      MonthFinance        `json:"last_12_months"`
	AnnualProjection  MonthFinance        `json:"annual_projection"`
	Series            []MonthFinance      `json:"series"`
	BySubstation      []SubstationFinance `json:"by_substation"`
	TopRecoverable    []RecoverableItem   `json:"top_recoverable"`
	CriticalShare     float64             `json:"critical_cost_share_pct"`
	TransformersTotal int                 `json:"transformers_total"`
}

// Managerial builds the managerial (financial) dashboard over 12 months.
func (uc *DashboardUseCase) Managerial(ctx context.Context, tenantID domain.UUID, tariffOverride float64) (*ManagerialDashboard, error) {
	tariff, source := uc.Tariff(ctx, tenantID, tariffOverride)
	now := time.Now()
	start := domain.MonthStart(now).AddDate(0, -11, 0)
	rows, err := uc.balanceRepo.ListRowsByTenantPeriod(ctx, tenantID, start, now)
	if err != nil {
		return nil, err
	}
	d := &ManagerialDashboard{GeneratedAt: now, TariffBRLMWh: tariff, TariffSource: source,
		Series: []MonthFinance{}, BySubstation: []SubstationFinance{}, TopRecoverable: []RecoverableItem{}}

	byMonth := map[string]*MonthFinance{}
	var months []string
	for _, r := range rows {
		key := periodMonth(r)
		m, ok := byMonth[key]
		if !ok {
			m = &MonthFinance{Month: key}
			byMonth[key] = m
			months = append(months, key)
		}
		m.add(r, tariff)
		d.Last12Months.add(r, tariff)
		if strings.HasPrefix(key, now.In(domain.BusinessLocation).Format("2006")) {
			d.YearToDate.add(r, tariff)
		}
	}
	sort.Strings(months)
	for _, k := range months {
		byMonth[k].finish()
		d.Series = append(d.Series, *byMonth[k])
	}
	d.Last12Months.finish()
	d.YearToDate.finish()
	if n := len(d.Series); n > 0 {
		d.CurrentMonth = &d.Series[n-1]
		if n > 1 {
			d.PreviousMonth = &d.Series[n-2]
			dpp := round2(d.CurrentMonth.LossPct - d.PreviousMonth.LossPct)
			d.DeltaLossPP = &dpp
			if d.PreviousMonth.LossCostBRL > 0 {
				v := round2((d.CurrentMonth.LossCostBRL - d.PreviousMonth.LossCostBRL) / d.PreviousMonth.LossCostBRL * 100)
				d.DeltaLossCostPct = &v
			}
		}
		// Projecao anual: media dos ultimos 3 meses com dados x 12
		k := min(3, n)
		var p MonthFinance
		for _, m := range d.Series[n-k:] {
			p.InjectedMWh += m.InjectedMWh
			p.BilledMWh += m.BilledMWh
			p.LossMWh += m.LossMWh
			p.ExcessLossMWh += m.ExcessLossMWh
			p.LossCostBRL += m.LossCostBRL
			p.RevenueBRL += m.RevenueBRL
			p.RecoverableBRL += m.RecoverableBRL
		}
		f := 12 / float64(k)
		p.InjectedMWh, p.BilledMWh, p.LossMWh, p.ExcessLossMWh = p.InjectedMWh*f, p.BilledMWh*f, p.LossMWh*f, p.ExcessLossMWh*f
		p.LossCostBRL, p.RevenueBRL, p.RecoverableBRL = p.LossCostBRL*f, p.RevenueBRL*f, p.RecoverableBRL*f
		p.finish()
		d.AnnualProjection = p

		// Ultimo mes: por subestacao e ranking de valor recuperavel
		last := d.Series[n-1].Month
		subs := map[string]*SubstationFinance{}
		var criticalCost, totalCost float64
		for _, r := range rows {
			if periodMonth(r) != last {
				continue
			}
			name := "Sem subestação"
			if r.SubstationName != nil && *r.SubstationName != "" {
				name = *r.SubstationName
			}
			s, ok := subs[name]
			if !ok {
				s = &SubstationFinance{Substation: name}
				subs[name] = s
			}
			s.add(r, tariff)
			limit := deref(r.LossLimitPct)
			excess := math.Max(0, r.LossKWh-r.EnergyInjectedKWh*limit/100)
			cost := r.LossKWh / 1000 * tariff
			totalCost += cost
			if r.Status == domain.BalanceStatusCritical {
				criticalCost += cost
			}
			if excess > 0 {
				d.TopRecoverable = append(d.TopRecoverable, RecoverableItem{
					TransformerID: r.TransformerID, Code: r.TransformerCode, Substation: r.SubstationName,
					LossPct: round2(r.LossPct), LossLimitPct: limit, ExcessLossKWh: round2(excess),
					LossCostBRL: round2(cost), RecoverableBRL: round2(excess / 1000 * tariff), Status: string(r.Status),
				})
			}
		}
		for _, s := range subs {
			s.finish()
			d.BySubstation = append(d.BySubstation, *s)
		}
		sort.Slice(d.BySubstation, func(i, j int) bool { return d.BySubstation[i].LossCostBRL > d.BySubstation[j].LossCostBRL })
		sort.Slice(d.TopRecoverable, func(i, j int) bool { return d.TopRecoverable[i].RecoverableBRL > d.TopRecoverable[j].RecoverableBRL })
		if len(d.TopRecoverable) > 10 {
			d.TopRecoverable = d.TopRecoverable[:10]
		}
		d.CriticalShare = pct(criticalCost, totalCost)
		d.TransformersTotal = d.CurrentMonth.Transformers
	}
	return d, nil
}

// periodMonth is the YYYY-MM of the middle of the balance period in
// BusinessLocation (robusto a periodos gravados em UTC ou em Brasilia).
func periodMonth(r *repository.BalanceRow) string {
	mid := r.PeriodStart.Add(r.PeriodEnd.Sub(r.PeriodStart) / 2)
	return mid.In(domain.BusinessLocation).Format("2006-01")
}
