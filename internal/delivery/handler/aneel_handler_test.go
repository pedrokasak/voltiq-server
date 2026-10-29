package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/voltiq/server/internal/aneel"
)

type fakeAneel struct {
	err error
	set *aneel.TariffSet
}

func (f fakeAneel) Distributors(context.Context, string) ([]aneel.Distributor, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]aneel.Distributor, 80)
	for i := range out {
		out[i] = aneel.Distributor{Agent: fmt.Sprintf("D%02d", i)}
	}
	return out, nil
}

func (f fakeAneel) CurrentTariff(context.Context, string) (*aneel.TariffSet, error) {
	return f.set, f.err
}

func call(h http.HandlerFunc, url string) (int, map[string]any) {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestAneelHandler(t *testing.T) {
	ok := NewAneelHandler(fakeAneel{set: &aneel.TariffSet{Agent: "CERILUZ", SuggestedBRLMWh: 699.45}})
	if code, body := call(ok.Tariff, "/aneel/tariffs?agent=CERILUZ"); code != http.StatusOK ||
		body["data"].(map[string]any)["suggested_brl_mwh"].(float64) != 699.45 {
		t.Fatalf("tariff: %d %v", code, body)
	}
	if code, _ := call(ok.Tariff, "/aneel/tariffs"); code != http.StatusUnprocessableEntity {
		t.Fatalf("missing agent: %d", code)
	}
	if code, body := call(ok.Distributors, "/aneel/distributors"); code != http.StatusOK || len(body["data"].([]any)) != 50 {
		t.Fatalf("distributors not capped at 50: %d", code)
	}
	if code, _ := call(NewAneelHandler(fakeAneel{}).Tariff, "/aneel/tariffs?agent=X"); code != http.StatusNotFound {
		t.Fatalf("no tariff: %d", code)
	}
	down := NewAneelHandler(fakeAneel{err: fmt.Errorf("%w: timeout dial tcp 10.0.0.1", aneel.ErrUnavailable)})
	code, body := call(down.Tariff, "/aneel/tariffs?agent=X")
	if code != http.StatusBadGateway {
		t.Fatalf("unavailable: %d", code)
	}
	if msg := body["error"].(map[string]any)["message"].(string); msg == "" || strings.Contains(msg, "10.0.0.1") {
		t.Fatalf("internal detail leaked: %q", msg)
	}
}
