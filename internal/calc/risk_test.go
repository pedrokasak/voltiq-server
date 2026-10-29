package calc

import (
	"testing"
	"time"

	"github.com/voltiq/server/internal/domain"
)

func f64(v float64) *float64 { return &v }

func balanceWith(lossPct, injected, consumption float64, at time.Time) *domain.TransformerBalance {
	return &domain.TransformerBalance{
		TransformerID: "t1", LossPct: lossPct, EnergyInjectedKWh: injected, TotalConsumptionKWh: consumption,
		PeriodStart: at, PeriodEnd: at.AddDate(0, 1, 0), CalculatedAt: at.AddDate(0, 1, 1),
	}
}

func TestCalculateRiskScore_Levels(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tr := &domain.Transformer{ID: "t1", LossLimitPct: f64(8)}

	low := CalculateRiskScore(balanceWith(2, 1000, 980, now), nil, tr, 5)
	if low.Level != "LOW" || low.TrendDirection != "STABLE" || low.LossLimitPct != 8 {
		t.Fatalf("score baixo inesperado: %+v", low)
	}
	if len(low.Factors) != 5 {
		t.Fatalf("esperados 5 fatores, obtidos %d", len(low.Factors))
	}

	cur := balanceWith(20, 1000, 800, now)
	cur.TechnicalLossKWh, cur.NonTechnicalLossKWh = f64(10), f64(190)
	hist := []*domain.TransformerBalance{
		balanceWith(8, 1000, 920, now.AddDate(0, -3, 0)),
		balanceWith(12, 1000, 880, now.AddDate(0, -2, 0)),
		balanceWith(16, 1000, 840, now.AddDate(0, -1, 0)),
	}
	high := CalculateRiskScore(cur, hist, tr, 250)
	if high.TrendDirection != "WORSENING" {
		t.Errorf("tendência deveria piorar: %s", high.TrendDirection)
	}
	if high.Level != "CRITICAL" && high.Level != "HIGH" {
		t.Errorf("nível alto esperado, obtido %s (%.2f)", high.Level, high.Score)
	}
	if high.OverloadPct != 150 {
		t.Errorf("sobrecarga = %.2f, esperado 150", high.OverloadPct)
	}
	if high.Score > 100 {
		t.Errorf("score acima de 100: %.2f", high.Score)
	}

	improving := []*domain.TransformerBalance{
		balanceWith(16, 1000, 840, now.AddDate(0, -3, 0)),
		balanceWith(12, 1000, 880, now.AddDate(0, -2, 0)),
		balanceWith(8, 1000, 920, now.AddDate(0, -1, 0)),
	}
	if got := CalculateRiskScore(balanceWith(5, 1000, 950, now), improving, tr, 10).TrendDirection; got != "IMPROVING" {
		t.Errorf("tendência deveria melhorar: %s", got)
	}

	// Sem limite cadastrado usa 10% e sem calculo a qualidade de dados e zero
	noLimit := &domain.Transformer{ID: "t2"}
	b := balanceWith(9, 1000, 910, now)
	b.CalculatedAt = time.Time{}
	r := CalculateRiskScore(b, nil, noLimit, 0)
	if r.LossLimitPct != 10 || r.Factors[4].Value != 0 {
		t.Errorf("padrões inesperados: limite %.1f qualidade %.1f", r.LossLimitPct, r.Factors[4].Value)
	}

	medium := CalculateRiskScore(balanceWith(9.5, 1000, 905, now), nil, tr, 10)
	if medium.Level != "MEDIUM" {
		t.Errorf("nível médio esperado, obtido %s (%.2f)", medium.Level, medium.Score)
	}
}

func TestDetectAnomalies(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	tr := &domain.Transformer{ID: "t1", LossLimitPct: f64(8)}

	if got := DetectAnomalies("t1", []*domain.TransformerBalance{balanceWith(5, 1, 1, now)}, tr); len(got) != 0 {
		t.Fatalf("um balanço não gera anomalias: %v", got)
	}

	types := func(list []Anomaly) map[string]string {
		m := map[string]string{}
		for _, a := range list {
			m[a.Type] = a.Severity
		}
		return m
	}

	// Pico de perda > 100% e queda de consumo > 50%
	got := types(DetectAnomalies("t1", []*domain.TransformerBalance{
		balanceWith(20, 1000, 400, now), balanceWith(9, 1000, 910, now.AddDate(0, -1, 0)),
	}, tr))
	if got["LOSS_SPIKE"] != "CRITICAL" || got["CONSUMPTION_DROP"] != "HIGH" {
		t.Errorf("anomalias inesperadas: %v", got)
	}

	// Pico moderado (50-100%) e queda moderada (30-50%)
	got = types(DetectAnomalies("t1", []*domain.TransformerBalance{
		balanceWith(15, 1000, 600, now), balanceWith(9, 1000, 910, now.AddDate(0, -1, 0)),
	}, tr))
	if got["LOSS_SPIKE"] != "HIGH" || got["CONSUMPTION_DROP"] != "MEDIUM" {
		t.Errorf("severidades moderadas inesperadas: %v", got)
	}

	// Consumo maior que a energia injetada
	got = types(DetectAnomalies("t1", []*domain.TransformerBalance{
		balanceWith(1, 1000, 1100, now), balanceWith(1, 1000, 1000, now.AddDate(0, -1, 0)),
	}, tr))
	if got["BALANCE_INCONSISTENCY"] != "HIGH" {
		t.Errorf("inconsistência não detectada: %v", got)
	}

	// Acima do limite em todos os periodos (>= 3)
	got = types(DetectAnomalies("t1", []*domain.TransformerBalance{
		balanceWith(12, 1000, 880, now), balanceWith(11, 1000, 890, now.AddDate(0, -1, 0)),
		balanceWith(10, 1000, 900, now.AddDate(0, -2, 0)),
	}, &domain.Transformer{ID: "t1"}))
	if got["PERSISTENT_OVER_LIMIT"] != "CRITICAL" {
		t.Errorf("perda persistente não detectada: %v", got)
	}

	// Estavel: nenhuma anomalia
	if list := DetectAnomalies("t1", []*domain.TransformerBalance{
		balanceWith(5, 1000, 950, now), balanceWith(5, 1000, 950, now.AddDate(0, -1, 0)),
	}, tr); len(list) != 0 {
		t.Errorf("série estável gerou anomalias: %v", list)
	}
}

func TestEnergyHelpers(t *testing.T) {
	if EnergyFromDemand(10, 720) != 7200 {
		t.Error("EnergyFromDemand")
	}
	if calculateLossPct(10, 0) != 0 {
		t.Error("calculateLossPct com energia zero")
	}
	if CalculateTechnicalLoss(nil) != nil {
		t.Error("CalculateTechnicalLoss(nil) deve ser nil")
	}
	res := CalculateTechnicalLoss(&domain.CalculationInput{
		Transformer: domain.Transformer{PowerKVA: 75, SecondaryVoltageV: 220, CoreLossKW: f64(0.33),
			WindingLossKW: f64(1.2), LossLimitPct: f64(8)},
		EnergyInjected: 12000, UCConsumptions: []float64{11000}, PeriodHours: 720,
	})
	if res == nil || res.LossKWh != 1000 {
		t.Errorf("resultado inesperado: %+v", res)
	}
}

func TestGenerateAnomalyIDUnique(t *testing.T) {
	seen := make(map[string]bool)
	for range 1000 {
		id := generateAnomalyID()
		if len(id) != len("anom_")+12 {
			t.Fatalf("unexpected id length: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate anomaly id %q", id)
		}
		seen[id] = true
	}
}
