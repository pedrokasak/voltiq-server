package calc

import (
	"math"

	"github.com/voltiq/server/internal/domain"
)

// RiskScore represents the calculated risk score for a transformer
type RiskScore struct {
	TransformerID   domain.UUID `json:"transformer_id"`
	Score           float64     `json:"score"`            // 0-100
	Level           string      `json:"level"`            // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	LossPct         float64     `json:"loss_pct"`         // Current loss percentage
	LossLimitPct    float64     `json:"loss_limit_pct"`   // Regulatory limit
	OverloadPct     float64     `json:"overload_pct"`     // How much over limit
	UCCount         int         `json:"uc_count"`         // Number of consumer units
	TrendDirection  string      `json:"trend_direction"`  // "IMPROVING", "STABLE", "WORSENING"
	LastCalculated  string      `json:"last_calculated"`  // ISO timestamp
	Factors         []RiskFactor `json:"factors"`         // Contributing factors
}

// RiskFactor represents a contributing factor to the risk score
type RiskFactor struct {
	Name        string  `json:"name"`
	Weight      float64 `json:"weight"`       // 0-1
	Value       float64 `json:"value"`        // Normalized 0-1
	Contribution float64 `json:"contribution"` // Weight * Value
	Description string  `json:"description"`
}

// Anomaly represents a detected anomaly in transformer data
type Anomaly struct {
	ID              string            `json:"id"`
	TransformerID   domain.UUID       `json:"transformer_id"`
	Type            string            `json:"type"`              // "LOSS_SPIKE", "CONSUMPTION_DROP", "BALANCE_INCONSISTENCY", "READING_GAP"
	Severity        string            `json:"severity"`          // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	DetectedAt      string            `json:"detected_at"`       // ISO timestamp
	PeriodStart     string            `json:"period_start"`      // ISO timestamp
	PeriodEnd       string            `json:"period_end"`        // ISO timestamp
	Description     string            `json:"description"`
	MetricValue     float64           `json:"metric_value"`
	ExpectedValue   float64           `json:"expected_value"`
	DeviationPct    float64           `json:"deviation_pct"`
	Details         map[string]any    `json:"details"`
}

// CalculateRiskScore calculates a risk score for a transformer based on multiple factors
// Factors considered:
// 1. Current loss percentage vs limit (weight: 0.40)
// 2. Trend of losses over last 3 periods (weight: 0.25)
// 3. Number of consumer units (weight: 0.15)
// 4. Technical vs non-technical loss ratio (weight: 0.10)
// 5. Data quality/completeness (weight: 0.10)
func CalculateRiskScore(
	currentBalance *domain.TransformerBalance,
	historicalBalances []*domain.TransformerBalance,
	transformer *domain.Transformer,
	ucCount int,
) RiskScore {
	factors := []RiskFactor{}

	// Factor 1: Loss percentage vs limit (40% weight)
	lossLimit := 10.0
	if transformer.LossLimitPct != nil {
		lossLimit = *transformer.LossLimitPct
	}

	lossRatio := 0.0
	if lossLimit > 0 {
		lossRatio = currentBalance.LossPct / lossLimit
	}
	// Normalize: 0 = 0% of limit, 1 = at limit, >1 = over limit
	lossFactorValue := math.Min(lossRatio, 2.0) / 2.0 // Cap at 2x limit
	factors = append(factors, RiskFactor{
		Name:         "loss_vs_limit",
		Weight:       0.40,
		Value:        lossFactorValue,
		Contribution: 0.40 * lossFactorValue,
		Description:  "Current loss percentage relative to ANEEL regulatory limit",
	})

	// Factor 2: Trend over last 3 periods (25% weight)
	trendFactorValue := 0.5 // Default stable
	trendDirection := "STABLE"
	if len(historicalBalances) >= 3 {
		// Calculate linear trend (simple slope)
		n := float64(len(historicalBalances))
		sumX, sumY, sumXY, sumXX := 0.0, 0.0, 0.0, 0.0
		for i, b := range historicalBalances {
			x := float64(i)
			y := b.LossPct
			sumX += x
			sumY += y
			sumXY += x * y
			sumXX += x * x
		}
		slope := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
		// Normalize slope: positive = worsening, negative = improving
		if slope > 0.5 {
			trendDirection = "WORSENING"
			trendFactorValue = math.Min(0.5+slope*0.2, 1.0)
		} else if slope < -0.5 {
			trendDirection = "IMPROVING"
			trendFactorValue = math.Max(0.5+slope*0.2, 0.0)
		}
	}
	factors = append(factors, RiskFactor{
		Name:         "loss_trend",
		Weight:       0.25,
		Value:        trendFactorValue,
		Contribution: 0.25 * trendFactorValue,
		Description:  "Trend of losses over last 3 periods",
	})

	// Factor 3: Consumer unit count (15% weight)
	// More UCs = more complex to monitor, slightly higher risk
	ucFactorValue := math.Min(float64(ucCount)/100.0, 1.0) // Normalize to 100 UCs
	factors = append(factors, RiskFactor{
		Name:         "uc_count",
		Weight:       0.15,
		Value:        ucFactorValue,
		Contribution: 0.15 * ucFactorValue,
		Description:  "Number of consumer units connected to transformer",
	})

	// Factor 4: Technical vs non-technical loss ratio (10% weight)
	// Higher non-technical losses indicate potential theft/metering issues
	techLossFactorValue := 0.5 // Default balanced
	if currentBalance.TechnicalLossKWh != nil && currentBalance.NonTechnicalLossKWh != nil {
		total := *currentBalance.TechnicalLossKWh + *currentBalance.NonTechnicalLossKWh
		if total > 0 {
			nonTechRatio := *currentBalance.NonTechnicalLossKWh / total
			// High non-technical = higher risk
			techLossFactorValue = nonTechRatio
		}
	}
	factors = append(factors, RiskFactor{
		Name:         "loss_composition",
		Weight:       0.10,
		Value:        techLossFactorValue,
		Contribution: 0.10 * techLossFactorValue,
		Description:  "Ratio of non-technical to total losses",
	})

	// Factor 5: Data quality (10% weight)
	// Based on whether we have recent balance data
	dataQualityValue := 1.0
	if currentBalance.CalculatedAt.IsZero() {
		dataQualityValue = 0.0
	} else {
		// If calculated more than 60 days ago, reduce quality
		daysSinceCalc := 30.0 // Placeholder - would calculate actual days
		if daysSinceCalc > 60 {
			dataQualityValue = math.Max(1.0-daysSinceCalc/180.0, 0.3)
		}
	}
	factors = append(factors, RiskFactor{
		Name:         "data_quality",
		Weight:       0.10,
		Value:        dataQualityValue,
		Contribution: 0.10 * dataQualityValue,
		Description:  "Freshness and completeness of balance data",
	})

	// Calculate final score (0-100)
	totalScore := 0.0
	for _, f := range factors {
		totalScore += f.Contribution
	}
	score := math.Min(totalScore*100, 100)

	// Determine risk level
	level := "LOW"
	if score >= 80 {
		level = "CRITICAL"
	} else if score >= 60 {
		level = "HIGH"
	} else if score >= 40 {
		level = "MEDIUM"
	}

	return RiskScore{
		TransformerID:  currentBalance.TransformerID,
		Score:          math.Round(score*100) / 100,
		Level:          level,
		LossPct:        currentBalance.LossPct,
		LossLimitPct:   lossLimit,
		OverloadPct:    math.Max(0, (currentBalance.LossPct-lossLimit)/lossLimit*100),
		UCCount:        ucCount,
		TrendDirection: trendDirection,
		LastCalculated: currentBalance.CalculatedAt.Format("2006-01-02T15:04:05Z"),
		Factors:        factors,
	}
}

// DetectAnomalies detects anomalies in transformer balance data
func DetectAnomalies(
	transformerID domain.UUID,
	balances []*domain.TransformerBalance,
	transformer *domain.Transformer,
) []Anomaly {
	anomalies := []Anomaly{}

	if len(balances) < 2 {
		return anomalies
	}

	// Sort by period start (most recent first)
	// Assuming balances are already sorted by period_start DESC

	latest := balances[0]
	previous := balances[1]

	// Anomaly 1: Sudden loss spike (>50% increase from previous period)
	if previous.LossPct > 0 {
		lossChangePct := ((latest.LossPct - previous.LossPct) / previous.LossPct) * 100
		if lossChangePct > 50 {
			severity := "HIGH"
			if lossChangePct > 100 {
				severity = "CRITICAL"
			}
			anomalies = append(anomalies, Anomaly{
				ID:            generateAnomalyID(),
				TransformerID: transformerID,
				Type:          "LOSS_SPIKE",
				Severity:      severity,
				DetectedAt:    latest.CalculatedAt.Format("2006-01-02T15:04:05Z"),
				PeriodStart:   latest.PeriodStart.Format("2006-01-02"),
				PeriodEnd:     latest.PeriodEnd.Format("2006-01-02"),
				Description:   "Loss percentage increased significantly compared to previous period",
				MetricValue:   latest.LossPct,
				ExpectedValue: previous.LossPct,
				DeviationPct:  lossChangePct,
				Details: map[string]any{
					"previous_loss_pct": previous.LossPct,
					"current_loss_pct":  latest.LossPct,
					"change_pct":        lossChangePct,
				},
			})
		}
	}

	// Anomaly 2: Consumption drop (>30% decrease from previous)
	if previous.TotalConsumptionKWh > 0 {
		consumptionChangePct := ((latest.TotalConsumptionKWh - previous.TotalConsumptionKWh) / previous.TotalConsumptionKWh) * 100
		if consumptionChangePct < -30 {
			severity := "MEDIUM"
			if consumptionChangePct < -50 {
				severity = "HIGH"
			}
			anomalies = append(anomalies, Anomaly{
				ID:            generateAnomalyID(),
				TransformerID: transformerID,
				Type:          "CONSUMPTION_DROP",
				Severity:      severity,
				DetectedAt:    latest.CalculatedAt.Format("2006-01-02T15:04:05Z"),
				PeriodStart:   latest.PeriodStart.Format("2006-01-02"),
				PeriodEnd:     latest.PeriodEnd.Format("2006-01-02"),
				Description:   "Significant drop in total consumption detected",
				MetricValue:   latest.TotalConsumptionKWh,
				ExpectedValue: previous.TotalConsumptionKWh,
				DeviationPct:  math.Abs(consumptionChangePct),
				Details: map[string]any{
					"previous_consumption_kwh": previous.TotalConsumptionKWh,
					"current_consumption_kwh":  latest.TotalConsumptionKWh,
					"change_pct":               consumptionChangePct,
				},
			})
		}
	}

	// Anomaly 3: Balance inconsistency (injected energy < consumption)
	if latest.EnergyInjectedKWh > 0 && latest.TotalConsumptionKWh > latest.EnergyInjectedKWh {
		excessPct := ((latest.TotalConsumptionKWh - latest.EnergyInjectedKWh) / latest.EnergyInjectedKWh) * 100
		anomalies = append(anomalies, Anomaly{
			ID:            generateAnomalyID(),
			TransformerID: transformerID,
			Type:          "BALANCE_INCONSISTENCY",
			Severity:      "HIGH",
			DetectedAt:    latest.CalculatedAt.Format("2006-01-02T15:04:05Z"),
			PeriodStart:   latest.PeriodStart.Format("2006-01-02"),
			PeriodEnd:     latest.PeriodEnd.Format("2006-01-02"),
			Description:   "Total consumption exceeds injected energy - possible meter error or unmetered generation",
			MetricValue:   latest.TotalConsumptionKWh,
			ExpectedValue: latest.EnergyInjectedKWh,
			DeviationPct:  excessPct,
			Details: map[string]any{
				"energy_injected_kwh":    latest.EnergyInjectedKWh,
				"total_consumption_kwh":  latest.TotalConsumptionKWh,
				"excess_kwh":             latest.TotalConsumptionKWh - latest.EnergyInjectedKWh,
			},
		})
	}

	// Anomaly 4: Reading gaps (if we have transformer readings data)
	// This would need transformer readings - placeholder for now

	// Anomaly 5: Loss consistently over limit
	overLimitCount := 0
	limit := 10.0
	if transformer.LossLimitPct != nil {
		limit = *transformer.LossLimitPct
	}
	for _, b := range balances {
		if b.LossPct >= limit {
			overLimitCount++
		}
	}
	if len(balances) >= 3 && overLimitCount == len(balances) {
		anomalies = append(anomalies, Anomaly{
			ID:            generateAnomalyID(),
			TransformerID: transformerID,
			Type:          "PERSISTENT_OVER_LIMIT",
			Severity:      "CRITICAL",
			DetectedAt:    latest.CalculatedAt.Format("2006-01-02T15:04:05Z"),
			PeriodStart:   balances[len(balances)-1].PeriodStart.Format("2006-01-02"),
			PeriodEnd:     latest.PeriodEnd.Format("2006-01-02"),
			Description:   "Transformer has been over loss limit for all recent periods",
			MetricValue:   latest.LossPct,
			ExpectedValue: limit,
			DeviationPct:  ((latest.LossPct - limit) / limit) * 100,
			Details: map[string]any{
				"consecutive_periods_over_limit": overLimitCount,
				"regulatory_limit_pct":           limit,
				"periods_analyzed":               len(balances),
			},
		})
	}

	return anomalies
}

func generateAnomalyID() string {
	return "anom_" + randomString(12)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[int(math.Mod(float64(i)*123457, float64(len(letters))))]
	}
	return string(b)
}