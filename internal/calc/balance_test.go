// Package calc provides basic balance calculation functions.
package calc_test

import (
	"testing"

	"github.com/energybalance/server/internal/calc"
	"github.com/energybalance/server/internal/domain"
)

func TestCalculateBasicBalance(t *testing.T) {
	tests := []struct {
		name     string
		injected float64
		consumed []float64
		expected float64
	}{
		{"positive_balance", 1000, []float64{800}, 200},
		{"negative_balance", 800, []float64{1000}, -200},
		{"zero_balance", 500, []float64{500}, 0},
		{"no_consumption", 1000, []float64{}, 1000},
		{"multiple_uc", 1000, []float64{400, 300, 200}, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.CalculateBasicBalance(tt.injected, tt.consumed)
			if result != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestCalculateLossPercentage(t *testing.T) {
	tests := []struct {
		name     string
		loss     float64
		injected float64
		expected float64
	}{
		{"loss_10_pct", 100, 1000, 10.0},
		{"loss_5_pct", 50, 1000, 5.0},
		{"zero_loss", 0, 1000, 0},
		{"zero_injected", 100, 0, 0},
		{"negative_loss", -50, 1000, -5.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.CalculateLossPercentage(tt.loss, tt.injected)
			if result != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestDetermineBalanceStatus(t *testing.T) {
	limit10 := 10.0
	limit5 := 5.0

	tests := []struct {
		name     string
		lossPct  float64
		limitPct *float64
		expected domain.BalanceCalculationStatus
	}{
		{"normal", 5.0, &limit10, domain.BalanceCalculationStatusNormal},
		{"warning", 9.0, &limit10, domain.BalanceCalculationStatusWarning},
		{"critical", 12.0, &limit10, domain.BalanceCalculationStatusCritical},
		{"limit_5", 4.5, &limit5, domain.BalanceCalculationStatusWarning},
		{"no_limit", 5.0, nil, domain.BalanceCalculationStatusNormal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.DetermineBalanceStatus(tt.lossPct, tt.limitPct)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCalculateTechnicalLossPRODIST(t *testing.T) {
	coreLoss := 0.5
	windingLoss := 1.2

	tests := []struct {
		name           string
		energyInjected float64
		powerKVA       float64
		coreLossKW     *float64
		windingLossKW  *float64
		periodHours    float64
		expected       float64
	}{
		{"full_calculation", 1000, 100, &coreLoss, &windingLoss, 720, 360.0 + 0.3*720},
		{"no_core_loss", 1000, 100, nil, &windingLoss, 720, 0},
		{"no_winding_loss", 1000, 100, &coreLoss, nil, 720, 0},
		{"zero_period", 1000, 100, &coreLoss, &windingLoss, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.CalculateTechnicalLossPRODIST(
				tt.energyInjected,
				tt.powerKVA,
				tt.coreLossKW,
				tt.windingLossKW,
				tt.periodHours,
			)
			if result < tt.expected-0.01 || result > tt.expected+0.01 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}
