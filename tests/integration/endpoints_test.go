package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/voltiq/server/internal/app"
	"github.com/voltiq/server/internal/domain"
)

// testApp is the production wiring served by httptest.
type testApp struct {
	*app.App
	Server *httptest.Server
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido: testes de integração ignorados")
	}
	a, err := app.Build(context.Background(), app.Options{
		JWTSecret: "integration-test-secret-0123456789abcdef", DisableRateLimit: true,
		CORSOrigins: []string{"http://localhost:5173"},
	})
	if err != nil {
		// Com DATABASE_URL definido a falha e real (no CI nao pode ser pulada)
		t.Fatalf("banco indisponível: %v", err)
	}
	ta := &testApp{App: a, Server: httptest.NewServer(a.Router)}
	t.Cleanup(func() {
		ta.Server.Close()
		a.Close()
	})
	return ta
}

// session is an authenticated client.
type session struct {
	t     *testing.T
	base  string
	token string
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
}

func (s *session) do(method, path string, body any) (int, envelope, []byte) {
	s.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, s.base+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	return s.send(req)
}

func (s *session) send(req *http.Request) (int, envelope, []byte) {
	s.t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	_ = json.Unmarshal(raw, &env)
	return resp.StatusCode, env, raw
}

func (s *session) upload(path, filename, content string, fields map[string]string) (int, envelope, []byte) {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write([]byte(content))
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, s.base+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+s.token)
	return s.send(req)
}

func expect(t *testing.T, label string, got, want int, raw []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: status %d, esperado %d; corpo: %s", label, got, want, string(raw))
	}
}

func decode[T any](t *testing.T, env envelope) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("decode %T: %v (%s)", v, err, string(env.Data))
	}
	return v
}

// signup creates a fresh tenant with a unique CNPJ and returns its owner session.
func (ta *testApp) signup(t *testing.T) (*session, string) {
	t.Helper()
	s := &session{t: t, base: ta.Server.URL + "/api/v1"}
	id := uuid.New().String()[:8]
	doc := fmt.Sprintf("%014d", time.Now().UnixNano()%1e14)
	code, env, raw := s.do(http.MethodPost, "/auth/signup", map[string]string{
		"company_name": "Cooperativa " + id, "cnpj": doc, "name": "Owner " + id,
		"email": "owner-" + id + "@example.com", "password": "senhaForte123",
	})
	expect(t, "signup", code, http.StatusCreated, raw)
	out := decode[struct {
		Token  string        `json:"token"`
		Tenant domain.Tenant `json:"tenant"`
	}](t, env)
	if out.Token == "" {
		t.Fatal("signup sem token")
	}
	s.token = out.Token
	return s, string(out.Tenant.ID)
}

func TestHealth(t *testing.T) {
	ta := newTestApp(t)
	resp, err := http.Get(ta.Server.URL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("health: %v %v", err, resp)
	}
}

// TestCORSPreflight garante que o navegador do app web passa no preflight
// (o cliente envia X-Requested-With e usa PATCH).
func TestCORSPreflight(t *testing.T) {
	ta := newTestApp(t)
	req, _ := http.NewRequest(http.MethodOptions, ta.Server.URL+"/api/v1/billing/plan", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-requested-with")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("preflight recusado: Allow-Origin=%q status=%d", got, resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers")), "x-requested-with") {
		t.Errorf("X-Requested-With não permitido: %q", resp.Header.Get("Access-Control-Allow-Headers"))
	}
}

func TestAuthFlow(t *testing.T) {
	ta := newTestApp(t)
	s, _ := ta.signup(t)

	code, env, raw := s.do(http.MethodGet, "/auth/me", nil)
	expect(t, "me", code, http.StatusOK, raw)
	me := decode[map[string]any](t, env)
	for _, k := range []string{"user", "tenant", "permissions", "limits", "usage", "onboarding"} {
		if _, ok := me[k]; !ok {
			t.Errorf("/auth/me sem %q", k)
		}
	}

	anon := &session{t: t, base: s.base}
	code, _, raw = anon.do(http.MethodGet, "/transformers", nil)
	expect(t, "sem token", code, http.StatusUnauthorized, raw)

	code, _, raw = anon.do(http.MethodPost, "/auth/login", map[string]string{"email": "nobody@example.com", "password": "x"})
	expect(t, "login inválido", code, http.StatusUnauthorized, raw)
}

// TestOperationalFlow cobre cadastro, importacao dinamica, balanco, mapa e
// dashboards operacional/gerencial.
func TestOperationalFlow(t *testing.T) {
	ta := newTestApp(t)
	s, _ := ta.signup(t)

	code, env, raw := s.do(http.MethodPost, "/substations", map[string]any{"code": "SE-01", "name": "Centro", "lat": -23.5, "lng": -46.6})
	expect(t, "subestação", code, http.StatusCreated, raw)
	sub := decode[domain.Substation](t, env)

	lat, lng := -23.55, -46.63
	code, env, raw = s.do(http.MethodPost, "/transformers", map[string]any{
		"code": "TRF-001", "power_kva": 75, "primary_voltage_kv": 13.8, "secondary_voltage_v": 220,
		"lat": lat, "lng": lng, "core_loss_kw": 0.33, "winding_loss_kw": 1.2, "loss_limit_pct": 8,
		"substation_id": sub.ID,
	})
	expect(t, "transformador", code, http.StatusCreated, raw)
	trafo := decode[domain.Transformer](t, env)

	code, _, raw = s.do(http.MethodPost, "/transformers", map[string]any{"code": "TRF-001", "power_kva": 75, "primary_voltage_kv": 13.8, "secondary_voltage_v": 220})
	expect(t, "código duplicado", code, http.StatusConflict, raw)
	code, _, raw = s.do(http.MethodPost, "/transformers", map[string]any{"code": "X", "power_kva": -1})
	expect(t, "validação", code, http.StatusUnprocessableEntity, raw)

	for _, uc := range []string{"UC-1", "UC-2"} {
		code, _, raw = s.do(http.MethodPost, "/consuming-units", map[string]any{
			"transformer_id": trafo.ID, "uc_code": uc, "name": "Cliente " + uc, "class": "residencial",
		})
		expect(t, "UC "+uc, code, http.StatusCreated, raw)
	}

	month := time.Now().AddDate(0, -1, 0)
	day := func(d int) string {
		return time.Date(month.Year(), month.Month(), d, 12, 0, 0, 0, time.UTC).Format("02/01/2006 15:04")
	}

	// Cabecalhos em portugues, separador ";" e decimal com virgula
	trafoCSV := "Data Leitura;Trafo;Energia (kWh);FP\n" +
		day(5) + ";TRF-001;6000,5;0,92\n" + day(20) + ";TRF-001;6100;0,93\n" + day(21) + ";TRF-404;10;0,9\n"
	code, env, raw = s.upload("/imports/validate", "leituras.csv", trafoCSV, map[string]string{"type": "transformer"})
	expect(t, "validar", code, http.StatusOK, raw)
	if !strings.Contains(string(env.Data), "TRF-404") {
		t.Errorf("validação deveria apontar trafo desconhecido: %s", env.Data)
	}

	code, env, raw = s.upload("/imports/transformers", "leituras.csv", trafoCSV, nil)
	expect(t, "importar trafo", code, http.StatusOK, raw)
	imp := decode[map[string]any](t, env)
	if imp["rows_ok"] == nil {
		t.Errorf("importação sem rows_ok: %v", imp)
	}

	ucCSV := "uc,data,consumo\nUC-1," + day(10) + ",5000\nUC-2," + day(10) + ",5500\n"
	code, _, raw = s.upload("/imports/ucs", "ucs.csv", ucCSV, map[string]string{"mapping": `{"reading_at":"data"}`})
	expect(t, "importar UCs", code, http.StatusOK, raw)

	// Reenvio do mesmo arquivo: duplicatas ignoradas
	code, env, raw = s.upload("/imports/transformers", "leituras.csv", trafoCSV, nil)
	expect(t, "reimportar", code, http.StatusOK, raw)
	if dup := decode[map[string]any](t, env)["duplicates_skipped"]; dup == nil || dup.(float64) < 2 {
		t.Errorf("duplicatas não detectadas: %v", dup)
	}

	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	code, env, raw = s.do(http.MethodPost, "/balance/transformer/"+string(trafo.ID)+"/calculate", map[string]string{
		"period_start": start.Format("2006-01-02"), "period_end": start.AddDate(0, 1, -1).Format("2006-01-02"),
	})
	expect(t, "balanço", code, http.StatusOK, raw)
	bal := decode[map[string]any](t, env)
	if !strings.Contains(string(env.Data), "loss_pct") {
		t.Errorf("balanço sem loss_pct: %v", bal)
	}

	code, env, raw = s.do(http.MethodGet, "/dashboard/map", nil)
	expect(t, "mapa", code, http.StatusOK, raw)
	m := decode[struct {
		Transformers []map[string]any `json:"transformers"`
		Substations  []map[string]any `json:"substations"`
		Bounds       map[string]any   `json:"bounds"`
	}](t, env)
	if len(m.Transformers) != 1 || len(m.Substations) != 1 || m.Bounds == nil {
		t.Fatalf("mapa incompleto: %s", env.Data)
	}
	if m.Transformers[0]["status"] == "NO_DATA" || m.Transformers[0]["loss_cost_brl"] == nil {
		t.Errorf("ponto do mapa sem balanço/valor: %v", m.Transformers[0])
	}

	for _, path := range []string{"/dashboard/operational", "/dashboard/managerial", "/dashboard/kpis",
		"/dashboard/kpi-sparklines", "/dashboard/loss-evolution", "/dashboard/status-distribution",
		"/dashboard/quarterly-injected-billed", "/dashboard/accumulated-loss-cost", "/dashboard/top-transformers",
		"/dashboard/financial-table", "/dashboard/transformer-current-status", "/dashboard/monthly-loss",
		"/dashboard/loss-composition", "/transformers?status=CRITICAL", "/imports", "/imports/summary",
		"/alerts", "/alerts/stats", "/substations", "/reports/comparative", "/tenant/settings", "/onboarding",
		"/transformers/" + string(trafo.ID) + "/loss-analysis", "/consuming-units",
	} {
		code, _, raw = s.do(http.MethodGet, path, nil)
		expect(t, path, code, http.StatusOK, raw)
	}

	code, env, raw = s.do(http.MethodGet, "/dashboard/managerial", nil)
	expect(t, "gerencial", code, http.StatusOK, raw)
	mg := decode[map[string]any](t, env)
	if mg["tariff_source"] != "default" || mg["current_month"] == nil {
		t.Errorf("gerencial inesperado: %s", env.Data)
	}

	// Tarifa do tenant passa a valer nos dashboards
	code, _, raw = s.do(http.MethodPut, "/tenant/settings", map[string]any{"tariff_brl_mwh": 600})
	expect(t, "tarifa", code, http.StatusOK, raw)
	_, env, _ = s.do(http.MethodGet, "/dashboard/managerial", nil)
	if decode[map[string]any](t, env)["tariff_brl_mwh"].(float64) != 600 {
		t.Errorf("tarifa do tenant ignorada: %s", env.Data)
	}

	// Leitura manual (RF02.5)
	code, _, raw = s.do(http.MethodPost, "/transformers/"+string(trafo.ID)+"/readings", map[string]any{
		"reading_at": time.Now().Add(-time.Hour).Format(time.RFC3339), "energy_kwh": 120.5,
	})
	expect(t, "leitura manual", code, http.StatusCreated, raw)

	for _, path := range []string{"/exports/balance/" + string(trafo.ID) + "?format=excel", "/exports/aneel",
		"/exports/readings?type=transformer", "/imports/template?type=transformers"} {
		req, _ := http.NewRequest(http.MethodGet, s.base+path, nil)
		req.Header.Set("Authorization", "Bearer "+s.token)
		code, _, raw = s.send(req)
		expect(t, path, code, http.StatusOK, raw[:min(len(raw), 300)])
	}
}

// TestRBACAndIsolation cobre convite, papel ENGINEER sem R$ e isolamento
// entre tenants.
func TestRBACAndIsolation(t *testing.T) {
	ta := newTestApp(t)
	owner, _ := ta.signup(t)

	code, env, raw := owner.do(http.MethodPost, "/transformers", map[string]any{
		"code": "TRF-A", "power_kva": 45, "primary_voltage_kv": 13.8, "secondary_voltage_v": 220,
	})
	expect(t, "trafo A", code, http.StatusCreated, raw)
	trafo := decode[domain.Transformer](t, env)

	email := "eng-" + uuid.New().String()[:8] + "@example.com"
	code, env, raw = owner.do(http.MethodPost, "/invites", map[string]string{"email": email, "role": "ENGINEER"})
	expect(t, "convite", code, http.StatusCreated, raw)
	token := decode[map[string]any](t, env)["token"].(string)

	eng := &session{t: t, base: owner.base}
	code, env, raw = eng.do(http.MethodPost, "/invites/"+token+"/accept", map[string]string{"name": "Eng", "password": "senhaForte123"})
	expect(t, "aceitar convite", code, http.StatusOK, raw)
	eng.token = decode[map[string]any](t, env)["token"].(string)

	code, _, raw = eng.do(http.MethodGet, "/dashboard/operational", nil)
	expect(t, "engenheiro operacional", code, http.StatusOK, raw)
	code, _, raw = eng.do(http.MethodGet, "/dashboard/managerial", nil)
	expect(t, "engenheiro gerencial", code, http.StatusForbidden, raw)
	code, _, raw = eng.do(http.MethodDelete, "/transformers/"+string(trafo.ID), nil)
	expect(t, "engenheiro exclui", code, http.StatusForbidden, raw)
	code, _, raw = eng.do(http.MethodGet, "/users", nil)
	expect(t, "engenheiro usuários", code, http.StatusForbidden, raw)
	code, env, raw = eng.do(http.MethodGet, "/dashboard/map", nil)
	expect(t, "engenheiro mapa", code, http.StatusOK, raw)
	if strings.Contains(string(env.Data), "tariff_brl_mwh") {
		t.Errorf("mapa expôs R$ ao engenheiro")
	}

	code, env, raw = owner.do(http.MethodGet, "/users", nil)
	expect(t, "usuários", code, http.StatusOK, raw)
	if !strings.Contains(string(env.Data), email) {
		t.Errorf("lista de usuários sem o engenheiro: %s", env.Data)
	}

	other, _ := ta.signup(t)
	code, _, raw = other.do(http.MethodGet, "/transformers/"+string(trafo.ID), nil)
	expect(t, "outro tenant lê", code, http.StatusNotFound, raw)
	code, _, raw = other.do(http.MethodPut, "/transformers/"+string(trafo.ID), map[string]any{"power_kva": 10})
	expect(t, "outro tenant altera", code, http.StatusNotFound, raw)
	code, _, raw = other.do(http.MethodDelete, "/transformers/"+string(trafo.ID), nil)
	expect(t, "outro tenant exclui", code, http.StatusNotFound, raw)
	code, env, raw = other.do(http.MethodGet, "/transformers", nil)
	expect(t, "outro tenant lista", code, http.StatusOK, raw)
	if strings.Contains(string(env.Data), "TRF-A") {
		t.Errorf("listagem vazou trafo de outro tenant")
	}
}

// TestSuperAdmin cobre o painel master e a gestao de assinaturas.
func TestSuperAdmin(t *testing.T) {
	ta := newTestApp(t)
	owner, tenantID := ta.signup(t)

	email := "super-" + uuid.New().String()[:8] + "@example.com"
	hash, _ := bcrypt.GenerateFromPassword([]byte("senhaForte123"), bcrypt.MinCost)
	now := time.Now()
	if err := ta.Users.Create(context.Background(), &domain.User{
		ID: domain.UUID(uuid.New().String()), TenantID: domain.UUID(tenantID), Email: email, Name: "Super",
		PasswordHash: string(hash), Role: domain.UserRoleSuperAdmin, Active: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("criar super admin: %v", err)
	}
	admin := &session{t: t, base: owner.base}
	code, env, raw := admin.do(http.MethodPost, "/auth/login", map[string]string{"email": email, "password": "senhaForte123"})
	expect(t, "login super", code, http.StatusOK, raw)
	admin.token = decode[map[string]any](t, env)["token"].(string)

	code, _, raw = owner.do(http.MethodGet, "/admin/tenants", nil)
	expect(t, "owner no admin", code, http.StatusForbidden, raw)

	code, env, raw = admin.do(http.MethodGet, "/admin/tenants?limit=5", nil)
	expect(t, "listar tenants", code, http.StatusOK, raw)
	if !strings.Contains(string(env.Data), `"total"`) {
		t.Errorf("lista sem total: %s", env.Data)
	}
	code, env, raw = admin.do(http.MethodGet, "/admin/tenants/"+tenantID, nil)
	expect(t, "detalhe", code, http.StatusOK, raw)
	if !strings.Contains(string(env.Data), `"usage"`) {
		t.Errorf("detalhe sem uso: %s", env.Data)
	}
	code, _, raw = admin.do(http.MethodPost, "/admin/tenants/"+tenantID+"/extend-trial", map[string]any{"days": 7, "reason": "teste"})
	expect(t, "estender trial", code, http.StatusOK, raw)
	code, _, raw = admin.do(http.MethodPatch, "/admin/tenants/"+tenantID+"/status", map[string]any{"status": "SUSPENDED", "reason": "teste"})
	expect(t, "suspender", code, http.StatusOK, raw)

	code, env, raw = owner.do(http.MethodGet, "/transformers", nil)
	expect(t, "tenant suspenso", code, http.StatusPaymentRequired, raw)
	if env.Error == nil || env.Error.Code != "TENANT_SUSPENDED" {
		t.Errorf("código inesperado: %s", raw)
	}
	code, _, raw = owner.do(http.MethodGet, "/auth/me", nil)
	expect(t, "perfil liberado", code, http.StatusOK, raw)

	code, _, raw = admin.do(http.MethodPost, "/admin/tenants/"+tenantID+"/activate", map[string]any{"plan": "pro"})
	expect(t, "ativar", code, http.StatusOK, raw)
	code, _, raw = owner.do(http.MethodGet, "/transformers", nil)
	expect(t, "tenant ativo", code, http.StatusOK, raw)

	code, env, raw = admin.do(http.MethodGet, "/admin/billing/metrics?period=90d", nil)
	expect(t, "métricas", code, http.StatusOK, raw)
	met := decode[map[string]any](t, env)
	for _, k := range []string{"mrr", "arr", "churn_rate", "ltv", "trial_conversion", "by_plan", "series"} {
		if _, ok := met[k]; !ok {
			t.Errorf("métricas sem %q", k)
		}
	}
	code, env, raw = admin.do(http.MethodGet, "/admin/audit", nil)
	expect(t, "auditoria", code, http.StatusOK, raw)
	if !strings.Contains(string(env.Data), "TENANT_ACTIVATED") {
		t.Errorf("auditoria sem ativação: %s", env.Data)
	}
}
