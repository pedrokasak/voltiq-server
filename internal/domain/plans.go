package domain

// PlanLimits are the quotas of a subscription plan. Zero means unlimited.
type PlanLimits struct {
	MaxUsers        int `json:"max_users"`
	MaxTransformers int `json:"max_transformers"`
}

// Limites por plano. Usuarios seguem os valores ja usados no billing
// (payment.DefaultPlanConfigs e super_admin_usecase); transformadores seguem
// o porte de cada faixa de cliente.
var planLimits = map[TenantPlan]PlanLimits{
	TenantPlanTrial:      {MaxUsers: 5, MaxTransformers: 50},
	TenantPlanStarter:    {MaxUsers: 10, MaxTransformers: 500},
	TenantPlanPro:        {MaxUsers: 50, MaxTransformers: 5000},
	TenantPlanEnterprise: {MaxUsers: 0, MaxTransformers: 0},
}

// LimitsForPlan returns the quotas of plan (trial limits for unknown plans).
func LimitsForPlan(plan TenantPlan) PlanLimits {
	if l, ok := planLimits[plan]; ok {
		return l
	}
	return planLimits[TenantPlanTrial]
}

// AllowsMore reports whether a quota with current usage accepts n more items.
func AllowsMore(max, current, n int) bool {
	return max <= 0 || current+n <= max
}
