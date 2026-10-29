// Carga de leitura dos dashboards (RNF01: p95 < 2 s com 50 usuarios).
// O rate limit (300/min por IP+UA+tenant) vale tambem aqui: rode contra
// staging ou com usuarios/origens distintas. Alternativa sem k6:
// go run ./scripts/load/loadtest (massa de dados em scripts/load/seed_50k.sql).
// Uso: k6 run -e BASE=https://voltiq-sw-server.fly.dev/api/v1 -e EMAIL=... -e PASSWORD=... scripts/load/k6-dashboard.js
import http from "k6/http";
import { check, sleep } from "k6";

export const options = {
  vus: 50,
  duration: "2m",
  thresholds: { http_req_duration: ["p(95)<2000"], http_req_failed: ["rate<0.01"] },
};

const BASE = __ENV.BASE || "http://localhost:8080/api/v1";

export function setup() {
  const res = http.post(`${BASE}/auth/login`, JSON.stringify({ email: __ENV.EMAIL, password: __ENV.PASSWORD }), {
    headers: { "Content-Type": "application/json" },
  });
  check(res, { "login 200": (r) => r.status === 200 });
  return { token: res.json("data.token") };
}

const paths = ["/dashboard/operational", "/dashboard/map", "/dashboard/kpis", "/transformers?limit=50", "/alerts/stats"];

export default function (data) {
  const params = { headers: { Authorization: `Bearer ${data.token}` } };
  for (const p of paths) {
    const r = http.get(`${BASE}${p}`, params);
    check(r, { [`${p} 200`]: (x) => x.status === 200 });
  }
  sleep(1);
}
