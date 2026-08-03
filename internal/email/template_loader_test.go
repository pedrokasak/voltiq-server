package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTemplateLoader(t *testing.T) {
	loader, err := NewTemplateLoader()
	require.NoError(t, err)
	assert.NotNil(t, loader)
	assert.NotEmpty(t, loader.templates)

	// Check that all 3 dunning templates were loaded
	assert.Contains(t, loader.templates, "dunning_stage_1")
	assert.Contains(t, loader.templates, "dunning_stage_2")
	assert.Contains(t, loader.templates, "dunning_stage_3")
}

func TestTemplateLoader_Render(t *testing.T) {
	loader, err := NewTemplateLoader()
	require.NoError(t, err)

	data := map[string]any{
		"tenant_name":   "Test Tenant",
		"plan_name":     "Plano Pro",
		"amount":        "R$ 299,00",
		"due_date":      "15/01/2026",
		"days_overdue":  7,
		"billing_url":   "https://app.voltiq.com.br/billing",
		"support_email": "suporte@voltiq.com.br",
		"company_name":  "Voltiq Software",
		"stage":         1,
		"current_year":  2026,
	}

	html, err := loader.Render("dunning_stage_1", data)
	require.NoError(t, err)
	assert.Contains(t, html, "Test Tenant")
	assert.Contains(t, html, "Plano Pro")
	assert.Contains(t, html, "R$ 299,00")
	assert.Contains(t, html, "1º aviso")
}

func TestTemplateLoader_Render_Stage2(t *testing.T) {
	loader, err := NewTemplateLoader()
	require.NoError(t, err)

	data := map[string]any{
		"tenant_name":   "Test Tenant",
		"plan_name":     "Plano Pro",
		"amount":        "R$ 299,00",
		"due_date":      "15/01/2026",
		"days_overdue":  7,
		"billing_url":   "https://app.voltiq.com.br/billing",
		"support_email": "suporte@voltiq.com.br",
		"company_name":  "Voltiq Software",
		"stage":         2,
		"current_year":  2026,
	}

	html, err := loader.Render("dunning_stage_2", data)
	require.NoError(t, err)
	assert.Contains(t, html, "2º aviso")
	assert.Contains(t, html, "Suspensão em 8 dias")
}

func TestTemplateLoader_Render_Stage3(t *testing.T) {
	loader, err := NewTemplateLoader()
	require.NoError(t, err)

	data := map[string]any{
		"tenant_name":   "Test Tenant",
		"plan_name":     "Plano Pro",
		"amount":        "R$ 299,00",
		"due_date":      "15/01/2026",
		"days_overdue":  15,
		"billing_url":   "https://app.voltiq.com.br/billing",
		"support_email": "suporte@voltiq.com.br",
		"company_name":  "Voltiq Software",
		"stage":         3,
		"current_year":  2026,
	}

	html, err := loader.Render("dunning_stage_3", data)
	require.NoError(t, err)
	assert.Contains(t, html, "ÚLTIMO AVISO")
	assert.Contains(t, html, "Suspensão automática amanhã")
}

func TestGetDunningTemplateName(t *testing.T) {
	tests := []struct {
		stage     int
		expected  string
	}{
		{1, "dunning_stage_1"},
		{2, "dunning_stage_2"},
		{3, "dunning_stage_3"},
		{99, "dunning_stage_1"}, // default
	}

	for _, tt := range tests {
		result := GetDunningTemplateName(tt.stage)
		assert.Equal(t, tt.expected, result)
	}
}