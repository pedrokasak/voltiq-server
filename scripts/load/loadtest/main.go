// Command loadtest mede latência dos endpoints de leitura sob concorrência
// (RNF01: p95 < 2 s com 50 usuários). Usa só a biblioteca padrão; cada usuário
// virtual tem User-Agent próprio, então o rate limit por IP+navegador não
// distorce a medição.
//
//	go run ./scripts/load/loadtest -base http://localhost:8080/api/v1 \
//	  -email admin@exemplo.com -password ... -vus 50 -duration 60s
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var defaultPaths = []string{
	"/dashboard/operational",
	"/dashboard/managerial",
	"/dashboard/map",
	"/dashboard/kpis",
	"/transformers?limit=50",
	"/transformers?limit=50&status=CRITICAL",
	"/alerts/stats",
}

type sample struct {
	path   string
	status int
	dur    time.Duration
}

func main() {
	base := flag.String("base", "http://localhost:8080/api/v1", "URL base da API")
	email := flag.String("email", "", "e-mail de login")
	password := flag.String("password", "", "senha")
	vus := flag.Int("vus", 50, "usuários virtuais")
	duration := flag.Duration("duration", 60*time.Second, "duração")
	think := flag.Duration("think", time.Second, "pausa entre iterações de cada usuário")
	paths := flag.String("paths", strings.Join(defaultPaths, ","), "endpoints separados por vírgula")
	p95Limit := flag.Duration("p95", 2*time.Second, "limite de p95 (falha acima)")
	flag.Parse()

	token, err := login(*base, *email, *password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "login:", err)
		os.Exit(1)
	}
	endpoints := strings.Split(*paths, ",")
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: *vus}}

	var mu sync.Mutex
	var samples []sample
	deadline := time.Now().Add(*duration)
	var wg sync.WaitGroup
	for v := range *vus {
		wg.Add(1)
		go func(vu int) {
			defer wg.Done()
			ua := fmt.Sprintf("voltiq-loadtest/vu-%d", vu)
			for time.Now().Before(deadline) {
				for _, p := range endpoints {
					s := get(client, *base+p, token, ua)
					s.path = p
					mu.Lock()
					samples = append(samples, s)
					mu.Unlock()
				}
				time.Sleep(*think)
			}
		}(v)
	}
	wg.Wait()

	if !report(samples, endpoints, *p95Limit) {
		os.Exit(1)
	}
}

func login(base, email, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	resp, err := http.Post(base+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || out.Data.Token == "" {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	return out.Data.Token, nil
}

func get(client *http.Client, url, token, ua string) sample {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", ua)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return sample{status: 0, dur: time.Since(start)}
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return sample{status: resp.StatusCode, dur: time.Since(start)}
}

func report(samples []sample, endpoints []string, p95Limit time.Duration) bool {
	ok := true
	fmt.Printf("%-42s %7s %7s %9s %9s %9s\n", "endpoint", "reqs", "erros", "p50", "p95", "max")
	for _, p := range append(endpoints, "TOTAL") {
		var durs []time.Duration
		errs := 0
		for _, s := range samples {
			if p != "TOTAL" && s.path != p {
				continue
			}
			durs = append(durs, s.dur)
			if s.status != http.StatusOK {
				errs++
			}
		}
		if len(durs) == 0 {
			continue
		}
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		p95 := durs[len(durs)*95/100]
		fmt.Printf("%-42s %7d %7d %9s %9s %9s\n", p, len(durs), errs,
			durs[len(durs)/2].Round(time.Millisecond), p95.Round(time.Millisecond), durs[len(durs)-1].Round(time.Millisecond))
		if p95 > p95Limit || errs > 0 {
			ok = false
		}
	}
	return ok
}
