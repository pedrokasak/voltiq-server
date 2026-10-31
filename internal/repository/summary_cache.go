package repository

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/reqctx"
)

// summaryTTL bounds how stale the cached network summaries can get for
// changes that do not invalidate it explicitly (leituras, alertas).
const summaryTTL = 15 * time.Second

// summaryCache keeps, per tenant, the full list of transformer summaries
// (ultimo balanco, UCs, ultima leitura, alertas). Os dashboards, o mapa e a
// lista de trafos leem a rede inteira a cada requisicao; com dezenas de
// milhares de trafos e varios usuarios simultaneos o banco saturava. A chave
// e o tenant_id, o mesmo filtro da consulta, entao um tenant nunca ve dados
// de outro. Requisicoes simultaneas do mesmo tenant compartilham um unico
// calculo.
type summaryCache struct {
	mu      sync.Mutex
	entries map[cacheKey]*summaryEntry
	gen     map[domain.UUID]uint64
}

// cacheKey separa os valores de um tenant (a lista de trafos e os
// dashboards montados a partir dela); invalidar o tenant vale para todos.
type cacheKey struct {
	tenant domain.UUID
	name   string
}

type summaryEntry struct {
	gen     uint64
	expires time.Time
	value   any
	err     error
	ready   chan struct{}
}

var summaries = &summaryCache{entries: map[cacheKey]*summaryEntry{}, gen: map[domain.UUID]uint64{}}

// get returns the cached summaries of the tenant, computing them with load
// when missing, expired or invalidated.
func (c *summaryCache) get(ctx context.Context, tenantID domain.UUID, load func(context.Context) ([]*TransformerSummary, error)) ([]*TransformerSummary, error) {
	v, err := c.memo(ctx, tenantID, "summaries", func(ctx context.Context) (any, error) { return load(ctx) })
	items, _ := v.([]*TransformerSummary)
	return items, err
}

// memo returns the cached value name of the tenant, computing it with load
// when missing, expired or invalidated; concurrent callers share one load.
func (c *summaryCache) memo(ctx context.Context, tenantID domain.UUID, name string, load func(context.Context) (any, error)) (any, error) {
	key := cacheKey{tenantID, name}
	c.mu.Lock()
	e := c.entries[key]
	gen := c.gen[tenantID]
	if e != nil && e.gen == gen {
		select {
		case <-e.ready:
			if e.err == nil && time.Now().Before(e.expires) {
				c.mu.Unlock()
				return e.value, nil
			}
		default:
			// calculo em andamento: espera o mesmo resultado
			c.mu.Unlock()
			select {
			case <-e.ready:
				return e.value, e.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	e = &summaryEntry{gen: gen, ready: make(chan struct{})}
	c.entries[key] = e
	c.purgeLocked()
	c.mu.Unlock()

	// Contexto desacoplado do cancelamento da requisicao que iniciou o
	// calculo (outras podem estar esperando), mantendo a identidade.
	value, err := load(context.WithoutCancel(ctx))
	e.value, e.err, e.expires = value, err, time.Now().Add(summaryTTL)
	close(e.ready)
	if err != nil {
		c.mu.Lock()
		if c.entries[key] == e {
			delete(c.entries, key)
		}
		c.mu.Unlock()
	}
	return value, err
}

// purgeLocked drops expired entries (c.mu held).
func (c *summaryCache) purgeLocked() {
	now := time.Now()
	for id, e := range c.entries {
		select {
		case <-e.ready:
			if now.After(e.expires) {
				delete(c.entries, id)
			}
		default:
		}
	}
}

// invalidate marks the tenant's summaries as stale. Sem tenant conhecido
// (jobs sem identidade), invalida todos.
func (c *summaryCache) invalidate(tenantID domain.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tenantID == "" {
		for k := range c.entries {
			c.gen[k.tenant]++
		}
		return
	}
	c.gen[tenantID]++
}

// InvalidateSummaries marks the cached summaries of the tenant as stale; use
// after writes that change balances, transformers or UCs outside the
// repositories that already do it.
func InvalidateSummaries(tenantID domain.UUID) { summaries.invalidate(tenantID) }

// Memo caches a value derived from the tenant's network data (dashboards,
// mapa) under name, with the same TTL and invalidation as the summaries. O
// valor devolvido e compartilhado entre requisicoes: nao pode ser alterado.
func Memo[T any](ctx context.Context, tenantID domain.UUID, name string, load func(context.Context) (T, error)) (T, error) {
	v, err := summaries.memo(ctx, tenantID, name, func(ctx context.Context) (any, error) { return load(ctx) })
	out, _ := v.(T)
	return out, err
}

// invalidateFromContext invalidates the tenant of the request (or all).
func invalidateFromContext(ctx context.Context) { summaries.invalidate(reqctx.TenantID(ctx)) }

// filterSummaries applies f (except GroupID, resolved in SQL) in memory and
// returns the page and the total, with the same ordering as the SQL path.
func filterSummaries(all []*TransformerSummary, f TransformerFilter) ([]*TransformerSummary, int) {
	search := strings.ToLower(f.Search)
	out := make([]*TransformerSummary, 0, len(all))
	for _, s := range all {
		if f.SubstationID != "" && (s.SubstationID == nil || string(*s.SubstationID) != f.SubstationID) {
			continue
		}
		if f.Active != nil && s.Active != *f.Active {
			continue
		}
		if f.Status != "" && f.Status != s.Status {
			switch f.Status {
			case "NORMAL", "WARNING", "CRITICAL", "NO_DATA":
				continue
			}
		}
		if search != "" && !strings.Contains(strings.ToLower(s.Code), search) &&
			(s.SubstationName == nil || !strings.Contains(strings.ToLower(*s.SubstationName), search)) &&
			(s.SubstationCode == nil || !strings.Contains(strings.ToLower(*s.SubstationCode), search)) {
			continue
		}
		out = append(out, s)
	}
	switch f.OrderBy {
	case "loss_pct":
		sort.SliceStable(out, func(i, j int) bool {
			a, b := out[i].LossPct, out[j].LossPct
			if (a == nil) != (b == nil) {
				return a != nil
			}
			if a != nil && *a != *b {
				return *a > *b
			}
			return out[i].Code < out[j].Code
		})
	case "power_kva":
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].PowerKVA != out[j].PowerKVA {
				return out[i].PowerKVA > out[j].PowerKVA
			}
			return out[i].Code < out[j].Code
		})
	case "updated_at":
		sort.SliceStable(out, func(i, j int) bool {
			if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
				return out[i].UpdatedAt.After(out[j].UpdatedAt)
			}
			return out[i].Code < out[j].Code
		})
	}
	total := len(out)
	if f.Limit > 0 {
		start := min(max(f.Offset, 0), total)
		out = out[start:min(start+f.Limit, total)]
	}
	return out, total
}
