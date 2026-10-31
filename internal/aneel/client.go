// Package aneel consulta o portal de dados abertos da ANEEL (CKAN,
// dadosabertos.aneel.gov.br): tarifas homologadas das distribuidoras e
// permissionárias (cooperativas), usadas como tarifa de referência do tenant.
package aneel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the ANEEL open data portal.
	DefaultBaseURL = "https://dadosabertos.aneel.gov.br"
	// DefaultTariffResourceID is the "Tarifas de Aplicação das Distribuidoras
	// de Energia Elétrica" datastore resource; configurável por
	// ANEEL_TARIFF_RESOURCE_ID caso a ANEEL republique o conjunto.
	DefaultTariffResourceID = "fcf2906c-7c32-4b9b-a637-054e7a5234f4"

	cacheTTL = 24 * time.Hour
)

// ErrUnavailable means the portal could not be queried (rede, HTTP ou
// resposta inesperada); o detalhe vai para o log, não para o usuário.
var ErrUnavailable = errors.New("dados abertos da ANEEL indisponíveis")

// Distributor is an agent (distribuidora ou permissionária) with tariffs.
type Distributor struct {
	Agent string `json:"agent"`
	CNPJ  string `json:"cnpj,omitempty"`
}

// TariffItem is one tariff line in force (R$/MWh).
type TariffItem struct {
	Class    string  `json:"class"`
	Subclass string  `json:"subclass"`
	Detail   string  `json:"detail"`
	Post     string  `json:"post"`
	Unit     string  `json:"unit"`
	TUSD     float64 `json:"tusd"`
	TE       float64 `json:"te"`
	Total    float64 `json:"total"`
}

// TariffSet is the B1 conventional tariff in force for one agent.
type TariffSet struct {
	Agent      string       `json:"agent"`
	Resolution string       `json:"resolution"`
	ValidFrom  string       `json:"valid_from"`
	ValidTo    string       `json:"valid_to"`
	Items      []TariffItem `json:"items"`
	// SuggestedBRLMWh is TUSD+TE of the residential class (referencia para o
	// custo da energia perdida), or the first item when there is none.
	SuggestedBRLMWh float64 `json:"suggested_brl_mwh"`
	Source          string  `json:"source"`
	FetchedAt       string  `json:"fetched_at"`
}

// Client queries the portal. Safe for concurrent use.
type Client struct {
	baseURL    string
	resourceID string
	http       *http.Client
	now        func() time.Time

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	value   any
	expires time.Time
}

// NewClient creates a client; empty arguments use the defaults.
func NewClient(baseURL, resourceID string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if resourceID == "" {
		resourceID = DefaultTariffResourceID
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), resourceID: resourceID, http: httpClient,
		now: time.Now, cache: map[string]cached{}}
}

type datastoreResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Records []map[string]any `json:"records"`
		Total   int              `json:"total"`
	} `json:"result"`
	Error any `json:"error"`
}

func (c *Client) search(ctx context.Context, params url.Values) ([]map[string]any, error) {
	params.Set("resource_id", c.resourceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/3/action/datastore_search?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	var out datastoreResponse
	if err := json.Unmarshal(body, &out); err != nil || !out.Success {
		return nil, fmt.Errorf("%w: resposta inválida (%v)", ErrUnavailable, err)
	}
	return out.Result.Records, nil
}

func (c *Client) cachedGet(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.cache[key]
	if !ok || c.now().After(v.expires) {
		return nil, false
	}
	return v.value, true
}

func (c *Client) cachedSet(key string, v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cached{value: v, expires: c.now().Add(cacheTTL)}
}

// Distributors lists the agents present in the tariff dataset, filtered by
// search (sigla ou CNPJ, sem diferenciar maiúsculas).
func (c *Client) Distributors(ctx context.Context, search string) ([]Distributor, error) {
	all, ok := c.cachedGet("distributors")
	if !ok {
		recs, err := c.search(ctx, url.Values{
			"fields":   {"SigAgente,NumCNPJDistribuidora"},
			"distinct": {"true"},
			"limit":    {"5000"},
			"sort":     {"SigAgente"},
		})
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		list := []Distributor{}
		for _, r := range recs {
			agent := str(r["SigAgente"])
			if agent == "" || seen[agent] {
				continue
			}
			seen[agent] = true
			list = append(list, Distributor{Agent: agent, CNPJ: digits(str(r["NumCNPJDistribuidora"]))})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Agent < list[j].Agent })
		c.cachedSet("distributors", list)
		all = list
	}
	list := all.([]Distributor)
	q := strings.ToLower(strings.TrimSpace(search))
	qd := digits(q)
	out := []Distributor{}
	for _, d := range list {
		if q == "" || strings.Contains(strings.ToLower(d.Agent), q) || (qd != "" && strings.Contains(d.CNPJ, qd)) {
			out = append(out, d)
		}
	}
	return out, nil
}

// CurrentTariff returns the B1 conventional "Tarifa de Aplicação" in force
// today for the agent (sigla exata, como em Distributors).
func (c *Client) CurrentTariff(ctx context.Context, agent string) (*TariffSet, error) {
	agent = strings.TrimSpace(agent)
	key := "tariff|" + strings.ToUpper(agent)
	if v, ok := c.cachedGet(key); ok {
		return v.(*TariffSet), nil
	}
	filters, _ := json.Marshal(map[string]string{
		"SigAgente":              agent,
		"DscBaseTarifaria":       "Tarifa de Aplicação",
		"DscSubGrupo":            "B1",
		"DscModalidadeTarifaria": "Convencional",
	})
	recs, err := c.search(ctx, url.Values{
		"filters": {string(filters)},
		"limit":   {"500"},
		"sort":    {"DatInicioVigencia desc"},
	})
	if err != nil {
		return nil, err
	}
	set := pickInForce(agent, recs, c.now())
	if set == nil {
		return nil, nil
	}
	set.Source = c.baseURL + "/dataset/tarifas-distribuidoras-energia-eletrica"
	set.FetchedAt = c.now().UTC().Format(time.RFC3339)
	c.cachedSet(key, set)
	return set, nil
}

// pickInForce keeps the records of the most recent resolution in force at
// now (vigência que contém a data; sem fim = vigente).
func pickInForce(agent string, recs []map[string]any, now time.Time) *TariffSet {
	today := now.Format("2006-01-02")
	type rec struct {
		start, end string
		r          map[string]any
	}
	var valid []rec
	for _, r := range recs {
		start, end := day(str(r["DatInicioVigencia"])), day(str(r["DatFimVigencia"]))
		if start == "" || start > today || (end != "" && end < today) {
			continue
		}
		valid = append(valid, rec{start, end, r})
	}
	if len(valid) == 0 {
		return nil
	}
	latest := valid[0].start
	for _, v := range valid {
		if v.start > latest {
			latest = v.start
		}
	}
	set := &TariffSet{Agent: agent, Items: []TariffItem{}}
	for _, v := range valid {
		if v.start != latest {
			continue
		}
		set.ValidFrom, set.ValidTo = v.start, v.end
		if set.Resolution == "" {
			set.Resolution = str(v.r["DscREH"])
		}
		tusd, te := num(v.r["VlrTUSD"]), num(v.r["VlrTE"])
		unit := str(v.r["DscUnidadeTerciaria"])
		if strings.EqualFold(unit, "kWh") { // normaliza para R$/MWh
			tusd, te = tusd*1000, te*1000
			unit = "MWh"
		}
		set.Items = append(set.Items, TariffItem{
			Class: str(v.r["DscClasse"]), Subclass: str(v.r["DscSubClasse"]), Detail: str(v.r["DscDetalhe"]),
			Post: str(v.r["NomPostoTarifario"]), Unit: unit,
			TUSD: round2(tusd), TE: round2(te), Total: round2(tusd + te),
		})
	}
	sort.SliceStable(set.Items, func(i, j int) bool {
		a, b := set.Items[i], set.Items[j]
		if a.Class != b.Class {
			return a.Class < b.Class
		}
		return a.Subclass < b.Subclass
	})
	// Preferencia: residencial sem detalhe (tarifa cheia), depois qualquer
	// residencial, depois o primeiro item.
	best := -1
	for i, it := range set.Items {
		if !strings.EqualFold(it.Class, "Residencial") {
			continue
		}
		plain := isNA(it.Detail) && (isNA(it.Subclass) || strings.EqualFold(it.Subclass, "Residencial"))
		if plain {
			best = i
			break
		}
		if best < 0 {
			best = i
		}
	}
	if best >= 0 {
		set.SuggestedBRLMWh = set.Items[best].Total
	}
	if set.SuggestedBRLMWh == 0 && len(set.Items) > 0 {
		set.SuggestedBRLMWh = set.Items[0].Total
	}
	return set
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// num parses values published as "123,45", "1.234,56", ",00" or numbers.
func num(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	s := str(v)
	if s == "" {
		return 0
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}
	if strings.HasPrefix(s, ".") {
		s = "0" + s
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// day normalizes "2026-04-22", "2026-04-22T00:00:00" or "22/04/2026".
func day(s string) string {
	if s == "" {
		return ""
	}
	if len(s) >= 10 && s[4] == '-' {
		return s[:10]
	}
	if t, err := time.Parse("02/01/2006", s[:min(len(s), 10)]); err == nil {
		return t.Format("2006-01-02")
	}
	return ""
}

func isNA(s string) bool {
	return s == "" || strings.EqualFold(s, "Não se aplica") || strings.EqualFold(s, "Nao se aplica")
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5*sign(v))) / 100
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}
