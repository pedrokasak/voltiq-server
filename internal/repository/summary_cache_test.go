package repository

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/voltiq/server/internal/domain"
)

func newTestCache() *summaryCache {
	return &summaryCache{entries: map[cacheKey]*summaryEntry{}, gen: map[domain.UUID]uint64{}}
}

func TestSummaryCacheSharesConcurrentLoads(t *testing.T) {
	c := newTestCache()
	var calls atomic.Int32
	release := make(chan struct{})
	load := func(context.Context) ([]*TransformerSummary, error) {
		calls.Add(1)
		<-release
		return []*TransformerSummary{{Transformer: domain.Transformer{Code: "T1"}}}, nil
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := c.get(context.Background(), "tenant-a", load)
			if err != nil || len(items) != 1 {
				t.Errorf("get: %v %v", items, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("load called %d times, want 1", n)
	}
	// cached
	if _, err := c.get(context.Background(), "tenant-a", load); err != nil || calls.Load() != 1 {
		t.Fatalf("expected cached result, calls=%d err=%v", calls.Load(), err)
	}
}

func TestSummaryCacheInvalidateAndIsolation(t *testing.T) {
	c := newTestCache()
	var calls atomic.Int32
	load := func(code string) func(context.Context) ([]*TransformerSummary, error) {
		return func(context.Context) ([]*TransformerSummary, error) {
			calls.Add(1)
			return []*TransformerSummary{{Transformer: domain.Transformer{Code: code}}}, nil
		}
	}
	a, _ := c.get(context.Background(), "tenant-a", load("A"))
	b, _ := c.get(context.Background(), "tenant-b", load("B"))
	if a[0].Code != "A" || b[0].Code != "B" {
		t.Fatalf("tenants mixed: %s %s", a[0].Code, b[0].Code)
	}
	c.invalidate("tenant-a")
	a, _ = c.get(context.Background(), "tenant-a", load("A2"))
	b, _ = c.get(context.Background(), "tenant-b", load("B2"))
	if a[0].Code != "A2" {
		t.Fatalf("tenant-a not reloaded after invalidate: %s", a[0].Code)
	}
	if b[0].Code != "B" {
		t.Fatalf("tenant-b should stay cached: %s", b[0].Code)
	}
	c.invalidate("") // sem tenant: todos
	b, _ = c.get(context.Background(), "tenant-b", load("B3"))
	if b[0].Code != "B3" {
		t.Fatalf("invalidate all did not reach tenant-b: %s", b[0].Code)
	}
}

func TestSummaryCacheDoesNotKeepErrorsOrExpired(t *testing.T) {
	c := newTestCache()
	fail := errors.New("db down")
	if _, err := c.get(context.Background(), "t", func(context.Context) ([]*TransformerSummary, error) { return nil, fail }); !errors.Is(err, fail) {
		t.Fatalf("want error, got %v", err)
	}
	items, err := c.get(context.Background(), "t", func(context.Context) ([]*TransformerSummary, error) {
		return []*TransformerSummary{{}}, nil
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("error was cached: %v %v", items, err)
	}
	c.entries[cacheKey{"t", "summaries"}].expires = time.Now().Add(-time.Second)
	items, _ = c.get(context.Background(), "t", func(context.Context) ([]*TransformerSummary, error) {
		return []*TransformerSummary{{}, {}}, nil
	})
	if len(items) != 2 {
		t.Fatalf("expired entry reused: %d", len(items))
	}
}

func f64p(v float64) *float64 { return &v }
func strp(v string) *string   { return &v }

func TestFilterSummaries(t *testing.T) {
	sub := domain.UUID("sub-1")
	all := []*TransformerSummary{
		{Transformer: domain.Transformer{Code: "TR-001", PowerKVA: 75, Active: true, SubstationID: &sub}, Status: "CRITICAL", LossPct: f64p(15), SubstationName: strp("Centro")},
		{Transformer: domain.Transformer{Code: "TR-002", PowerKVA: 30, Active: true}, Status: "NORMAL", LossPct: f64p(4)},
		{Transformer: domain.Transformer{Code: "TR-003", PowerKVA: 45, Active: false}, Status: "NO_DATA"},
		{Transformer: domain.Transformer{Code: "TR-004", PowerKVA: 112.5, Active: true, SubstationID: &sub}, Status: "WARNING", LossPct: f64p(9), SubstationName: strp("Centro")},
	}
	codes := func(items []*TransformerSummary) string {
		s := ""
		for _, i := range items {
			s += i.Code + " "
		}
		return s
	}
	active := true
	cases := []struct {
		name  string
		f     TransformerFilter
		want  string
		total int
	}{
		{"all", TransformerFilter{}, "TR-001 TR-002 TR-003 TR-004 ", 4},
		{"status", TransformerFilter{Status: "CRITICAL"}, "TR-001 ", 1},
		{"no data", TransformerFilter{Status: "NO_DATA"}, "TR-003 ", 1},
		{"unknown status ignored like SQL", TransformerFilter{Status: "X"}, "TR-001 TR-002 TR-003 TR-004 ", 4},
		{"substation", TransformerFilter{SubstationID: "sub-1"}, "TR-001 TR-004 ", 2},
		{"search code", TransformerFilter{Search: "tr-00"}, "TR-001 TR-002 TR-003 TR-004 ", 4},
		{"search substation", TransformerFilter{Search: "centro"}, "TR-001 TR-004 ", 2},
		{"active", TransformerFilter{Active: &active}, "TR-001 TR-002 TR-004 ", 3},
		{"loss desc nulls last", TransformerFilter{OrderBy: "loss_pct"}, "TR-001 TR-004 TR-002 TR-003 ", 4},
		{"power desc", TransformerFilter{OrderBy: "power_kva"}, "TR-004 TR-001 TR-003 TR-002 ", 4},
		{"page", TransformerFilter{Limit: 2, Offset: 1}, "TR-002 TR-003 ", 4},
		{"page past end", TransformerFilter{Limit: 2, Offset: 10}, "", 4},
	}
	for _, tc := range cases {
		got, total := filterSummaries(all, tc.f)
		if codes(got) != tc.want || total != tc.total {
			t.Errorf("%s: got %q (%d), want %q (%d)", tc.name, codes(got), total, tc.want, tc.total)
		}
	}
	if all[0].Code != "TR-001" || all[3].Code != "TR-004" {
		t.Fatal("filterSummaries reordered the cached slice")
	}
}
