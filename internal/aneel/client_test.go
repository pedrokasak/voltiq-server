package aneel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fakeCKAN imita o datastore_search do portal da ANEEL.
func fakeCKAN(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/3/action/datastore_search" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("resource_id") != "res-1" {
			t.Errorf("resource_id = %q", q.Get("resource_id"))
		}
		var records []map[string]any
		if q.Get("distinct") == "true" {
			records = []map[string]any{
				{"SigAgente": "CERILUZ", "NumCNPJDistribuidora": "87.656.989/0001-14"},
				{"SigAgente": "CEMIG-D", "NumCNPJDistribuidora": "06981180000116"},
				{"SigAgente": "CERILUZ", "NumCNPJDistribuidora": "87656989000114"},
			}
		} else {
			var f map[string]string
			if err := json.Unmarshal([]byte(q.Get("filters")), &f); err != nil {
				t.Errorf("filters: %v", err)
			}
			if f["SigAgente"] == "CERILUZ" && f["DscSubGrupo"] == "B1" && f["DscModalidadeTarifaria"] == "Convencional" {
				records = []map[string]any{
					// resolução vigente
					{"DscREH": "REH 3.400/2026", "DatInicioVigencia": "2026-06-01", "DatFimVigencia": "2027-05-31",
						"DscClasse": "Residencial", "DscSubClasse": "Residencial", "DscDetalhe": "Não se aplica",
						"NomPostoTarifario": "Não se aplica", "DscUnidadeTerciaria": "MWh", "VlrTUSD": "412,35", "VlrTE": "287,10"},
					{"DscREH": "REH 3.400/2026", "DatInicioVigencia": "2026-06-01", "DatFimVigencia": "2027-05-31",
						"DscClasse": "Residencial", "DscSubClasse": "Residencial", "DscDetalhe": "SCEE",
						"NomPostoTarifario": "Não se aplica", "DscUnidadeTerciaria": "MWh", "VlrTUSD": "300,00", "VlrTE": ",00"},
					{"DscREH": "REH 3.400/2026", "DatInicioVigencia": "2026-06-01", "DatFimVigencia": "2027-05-31",
						"DscClasse": "Rural", "DscSubClasse": "Agropecuária rural", "DscDetalhe": "Não se aplica",
						"NomPostoTarifario": "Não se aplica", "DscUnidadeTerciaria": "kWh", "VlrTUSD": "0,30123", "VlrTE": 0.2},
					// resolução anterior (vencida)
					{"DscREH": "REH 3.100/2025", "DatInicioVigencia": "2025-06-01", "DatFimVigencia": "2026-05-31",
						"DscClasse": "Residencial", "DscSubClasse": "Residencial", "DscDetalhe": "Não se aplica",
						"DscUnidadeTerciaria": "MWh", "VlrTUSD": "1.000,00", "VlrTE": "1,00"},
					// futura
					{"DscREH": "REH 3.900/2027", "DatInicioVigencia": "2027-06-01", "DatFimVigencia": "",
						"DscClasse": "Residencial", "DscSubClasse": "Residencial", "DscDetalhe": "Não se aplica",
						"DscUnidadeTerciaria": "MWh", "VlrTUSD": "999,00", "VlrTE": "1,00"},
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"records": records, "total": len(records)}})
	}))
}

func newTestClient(srv *httptest.Server) *Client {
	c := NewClient(srv.URL, "res-1", srv.Client())
	c.now = func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) }
	return c
}

func TestCurrentTariffPicksResolutionInForce(t *testing.T) {
	var calls atomic.Int32
	srv := fakeCKAN(t, &calls)
	defer srv.Close()
	c := newTestClient(srv)

	set, err := c.CurrentTariff(context.Background(), "CERILUZ")
	if err != nil || set == nil {
		t.Fatalf("CurrentTariff: %v %v", set, err)
	}
	if set.Resolution != "REH 3.400/2026" || set.ValidFrom != "2026-06-01" || set.ValidTo != "2027-05-31" {
		t.Fatalf("wrong resolution: %+v", set)
	}
	if len(set.Items) != 3 {
		t.Fatalf("items = %d, want 3 (only the resolution in force)", len(set.Items))
	}
	if set.SuggestedBRLMWh != 699.45 {
		t.Fatalf("suggested = %v, want 699.45 (residencial sem detalhe)", set.SuggestedBRLMWh)
	}
	var rural *TariffItem
	for i := range set.Items {
		if set.Items[i].Class == "Rural" {
			rural = &set.Items[i]
		}
	}
	if rural == nil || rural.Unit != "MWh" || rural.TUSD != 301.23 || rural.TE != 200 || rural.Total != 501.23 {
		t.Fatalf("kWh not normalized to MWh: %+v", rural)
	}

	// cache de 24 h
	before := calls.Load()
	if _, err := c.CurrentTariff(context.Background(), "ceriluz"); err != nil || calls.Load() != before {
		t.Fatalf("expected cached tariff (calls %d -> %d, err %v)", before, calls.Load(), err)
	}
}

func TestCurrentTariffUnknownAgent(t *testing.T) {
	var calls atomic.Int32
	srv := fakeCKAN(t, &calls)
	defer srv.Close()
	set, err := newTestClient(srv).CurrentTariff(context.Background(), "NAOEXISTE")
	if err != nil || set != nil {
		t.Fatalf("want nil, nil; got %v %v", set, err)
	}
}

func TestDistributorsDedupAndSearch(t *testing.T) {
	var calls atomic.Int32
	srv := fakeCKAN(t, &calls)
	defer srv.Close()
	c := newTestClient(srv)

	all, err := c.Distributors(context.Background(), "")
	if err != nil || len(all) != 2 || all[0].Agent != "CEMIG-D" || all[1].Agent != "CERILUZ" {
		t.Fatalf("distributors: %+v %v", all, err)
	}
	if all[1].CNPJ != "87656989000114" {
		t.Fatalf("cnpj not normalized: %q", all[1].CNPJ)
	}
	byName, _ := c.Distributors(context.Background(), "ceri")
	byCNPJ, _ := c.Distributors(context.Background(), "06.981.180")
	if len(byName) != 1 || byName[0].Agent != "CERILUZ" || len(byCNPJ) != 1 || byCNPJ[0].Agent != "CEMIG-D" {
		t.Fatalf("search failed: %+v %+v", byName, byCNPJ)
	}
	if calls.Load() != 1 {
		t.Fatalf("distributor list not cached: %d calls", calls.Load())
	}
}

func TestUnavailablePortal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := newTestClient(srv).CurrentTariff(context.Background(), "X")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success": false, "error": {"message": "x"}}`))
	}))
	defer bad.Close()
	if _, err := newTestClient(bad).Distributors(context.Background(), ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable for success=false, got %v", err)
	}
}

func TestParsers(t *testing.T) {
	cases := map[string]float64{"123,45": 123.45, "1.234,56": 1234.56, ",00": 0, "0.5": 0.5, "": 0, "abc": 0}
	for in, want := range cases {
		if got := num(in); got != want {
			t.Errorf("num(%q) = %v, want %v", in, got, want)
		}
	}
	if day("22/04/2026") != "2026-04-22" || day("2026-04-22T00:00:00") != "2026-04-22" || day("x") != "" {
		t.Error("day parser")
	}
}
