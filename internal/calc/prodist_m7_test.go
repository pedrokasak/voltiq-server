// Package calc_test implements unit tests for the calculation engine.
// All functions in internal/calc/ must have corresponding tests per AGENTS.md
package calc_test

import (
	"math"
	"testing"
	"time"

	"github.com/energybalance/server/internal/calc"
	"github.com/energybalance/server/internal/domain"
)

func ptrFloat(v float64) *float64 {
	return &v
}

func baseTransformer() domain.Transformer {
	return domain.Transformer{
		ID:                "trafo-001",
		TenantID:          "tenant-001",
		Code:              "TR-001",
		PowerKVA:          75,
		PrimaryVoltageKV:  13.8,
		SecondaryVoltageV: 220,
		CoreLossKW:        ptrFloat(0.265),
		WindingLossKW:     ptrFloat(1.350),
		LossLimitPct:      ptrFloat(10.0),
		Active:            true,
	}
}

func TestCalculateBalance_NormalCase(t *testing.T) {
	input := domain.CalculationInput{
		Transformer:    baseTransformer(),
		EnergyInjected: 10000.0,
		UCConsumptions: []float64{1500, 1200, 800, 600, 400, 300, 200, 150, 100, 50},
		PeriodHours:    720,
		AverageCurrent: 45.0,
	}
	result, err := calc.CalculateBalance(input)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	if !approx(result.LossKWh, 4700.0, 0.01) {
		t.Errorf("LossKWh: expected 4700.00, got %.4f", result.LossKWh)
	}
	if !approx(result.LossPct, 47.0, 0.01) {
		t.Errorf("LossPct: expected 47.00%%, got %.4f%%", result.LossPct)
	}
	if result.Status != domain.BalanceCalculationStatus(domain.BalanceStatusCritical) {
		t.Errorf("Status: expected CRITICAL, got %s", result.Status)
	}
	if result.UCCount != 10 {
		t.Errorf("UCCount: expected 10, got %d", result.UCCount)
	}
}

func TestCalculateBalance_StatusNormal(t *testing.T) {
	input := domain.CalculationInput{
		Transformer:    baseTransformer(),
		EnergyInjected: 10000,
		UCConsumptions: []float64{9500},
		PeriodHours:    720,
		AverageCurrent: 45,
	}
	result, err := calc.CalculateBalance(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.BalanceCalculationStatus(domain.BalanceStatusNormal) {
		t.Errorf("expected NORMAL, got %s", result.Status)
	}
}

func TestCalculateBalance_StatusWarning(t *testing.T) {
	input := domain.CalculationInput{
		Transformer:    baseTransformer(),
		EnergyInjected: 10000,
		UCConsumptions: []float64{9150},
		PeriodHours:    720,
		AverageCurrent: 45,
	}
	result, err := calc.CalculateBalance(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.BalanceCalculationStatus(domain.BalanceStatusWarning) {
		t.Errorf("expected WARNING, got %s", result.Status)
	}
}

func TestCalculateBalance_StatusCritical(t *testing.T) {
	input := domain.CalculationInput{
		Transformer:    baseTransformer(),
		EnergyInjected: 10000,
		UCConsumptions: []float64{8800},
		PeriodHours:    720,
		AverageCurrent: 45,
	}
	result, err := calc.CalculateBalance(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.BalanceCalculationStatus(domain.BalanceStatusCritical) {
		t.Errorf("expected CRITICAL, got %s", result.Status)
	}
}

func TestCalculateBalance_TechnicalLossTrafo_PRODIST_M7(t *testing.T) {
	transformer := baseTransformer()
	in := transformer.NominalCurrent()
	ic := in * 0.5

	input := domain.CalculationInput{
		Transformer:    transformer,
		EnergyInjected: 5000,
		UCConsumptions: []float64{4500},
		PeriodHours:    720,
		AverageCurrent: ic,
	}
	result, err := calc.CalculateBalance(input)
	if err != nil {
		t.Fatal(err)
	}

	expected := 0.265*720 + 1.350*math.Pow(0.5, 2)*720
	if !approx(result.TechnicalLossTrafoKWh, expected, 0.5) {
		t.Errorf("TechnicalLossTrafoKWh: expected %.2f, got %.4f", expected, result.TechnicalLossTrafoKWh)
	}
}

func TestCalculateBalance_NonTechnicalLossNonNegative(t *testing.T) {
	transformer := baseTransformer()
	transformer.CoreLossKW = ptrFloat(5.0)
	transformer.WindingLossKW = ptrFloat(5.0)
	input := domain.CalculationInput{
		Transformer:    transformer,
		EnergyInjected: 1000,
		UCConsumptions: []float64{998},
		PeriodHours:    720,
		AverageCurrent: 10,
	}
	result, err := calc.CalculateBalance(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.NonTechnicalLossKWh < 0 {
		t.Errorf("PNT cannot be negative, got %.4f", result.NonTechnicalLossKWh)
	}
}

func TestCalculateBalance_NoUCs(t *testing.T) {
	input := domain.CalculationInput{
		Transformer:    baseTransformer(),
		EnergyInjected: 5000,
		UCConsumptions: []float64{},
		PeriodHours:    720,
		AverageCurrent: 0,
	}
	result, err := calc.CalculateBalance(input)
	if err != nil {
		t.Fatal(err)
	}
	if !approx(result.LossKWh, 5000, 0.01) {
		t.Errorf("LossKWh without UCs: expected 5000, got %.4f", result.LossKWh)
	}
}

func TestCalculateBalance_ErrorEnergyZero(t *testing.T) {
	input := domain.CalculationInput{
		Transformer:    baseTransformer(),
		EnergyInjected: 0,
		UCConsumptions: []float64{100},
		PeriodHours:    720,
	}
	_, err := calc.CalculateBalance(input)
	if err != calc.ErrEnergyInjectedZero {
		t.Errorf("expected ErrEnergyInjectedZero, got: %v", err)
	}
}

func TestCalculateBalance_ErrorPeriodZero(t *testing.T) {
	input := domain.CalculationInput{
		Transformer:    baseTransformer(),
		EnergyInjected: 1000,
		UCConsumptions: []float64{900},
		PeriodHours:    0,
	}
	_, err := calc.CalculateBalance(input)
	if err != calc.ErrPeriodInvalid {
		t.Errorf("expected ErrPeriodInvalid, got: %v", err)
	}
}

func TestCalculateBalance_ErrorNameplateData(t *testing.T) {
	transformer := baseTransformer()
	transformer.CoreLossKW = nil
	input := domain.CalculationInput{
		Transformer:    transformer,
		EnergyInjected: 1000,
		UCConsumptions: []float64{900},
		PeriodHours:    720,
	}
	_, err := calc.CalculateBalance(input)
	if err != calc.ErrNameplateDataMissing {
		t.Errorf("expected ErrNameplateDataMissing, got: %v", err)
	}
}

func TestCalculateBalance_Batch(t *testing.T) {
	transformer := baseTransformer()
	inputs := []domain.CalculationInput{
		{Transformer: transformer, EnergyInjected: 10000, UCConsumptions: []float64{9500}, PeriodHours: 720, AverageCurrent: 45},
		{Transformer: transformer, EnergyInjected: 8000, UCConsumptions: []float64{6800}, PeriodHours: 720, AverageCurrent: 38},
		{Transformer: transformer, EnergyInjected: 12000, UCConsumptions: []float64{10500}, PeriodHours: 720, AverageCurrent: 60},
	}
	results, errs := calc.CalculateBalanceBatch(inputs)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, err := range errs {
		if err != nil {
			t.Errorf("input[%d]: unexpected error: %v", i, err)
		}
	}
	if !approx(results[0].EnergyInjectedKWh, 10000, 0.01) {
		t.Errorf("batch[0] energy: expected 10000, got %.2f", results[0].EnergyInjectedKWh)
	}
}

func TestPeriodInHours(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	hours := calc.PeriodInHours(start, end)
	if !approx(hours, 720.0, 0.01) {
		t.Errorf("PeriodInHours: expected 720.0, got %.2f", hours)
	}
}

func TestLoadFactor(t *testing.T) {
	cases := []struct {
		ic, in, expected float64
	}{
		{100, 200, 0.5},
		{200, 200, 1.0},
		{250, 200, 1.0},
		{0, 200, 0.0},
		{100, 0, 0.0},
	}
	for _, c := range cases {
		got := calc.LoadFactor(c.ic, c.in)
		if !approx(got, c.expected, 0.001) {
			t.Errorf("LoadFactor(%.0f, %.0f): expected %.3f, got %.3f", c.ic, c.in, c.expected, got)
		}
	}
}

func TestNominalCurrent_75kVA_220V(t *testing.T) {
	transformer := baseTransformer()
	in := transformer.NominalCurrent()
	if !approx(in, 196.83, 0.5) {
		t.Errorf("NominalCurrent: expected ≈196.83A, got %.2fA", in)
	}
}

func approx(got, expected, tol float64) bool {
	return math.Abs(got-expected) <= tol
}
