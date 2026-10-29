-- Massa de dados para teste de carga (RNF01/RNF04.3): 50 mil trafos, 2 UCs por
-- trafo, 6 meses de leituras e balanços para um tenant existente.
-- NÃO rodar em produção. Uso:
--   psql "$DATABASE_URL" -v tenant_id=<uuid> -v trafos=50000 -f scripts/load/seed_50k.sql
\set ON_ERROR_STOP 1
BEGIN;
SELECT set_config('app.tenant_id', :'tenant_id', true);
SELECT set_config('app.current_tenant_id', :'tenant_id', true);

INSERT INTO substations (tenant_id, code, name, lat, lng)
SELECT :'tenant_id'::uuid, 'SE-' || lpad(s::text, 3, '0'), 'Subestação ' || s,
       -19.9 + (s % 10) * 0.3, -43.9 + (s / 10) * 0.3
FROM generate_series(1, 40) s;

INSERT INTO transformers (tenant_id, substation_id, code, power_kva, primary_voltage_kv,
                          secondary_voltage_v, lat, lng, core_loss_kw, winding_loss_kw, loss_limit_pct)
SELECT :'tenant_id'::uuid,
       (SELECT id FROM substations WHERE tenant_id = :'tenant_id'::uuid
         AND code = 'SE-' || lpad((1 + t % 40)::text, 3, '0')),
       'TR-' || lpad(t::text, 6, '0'),
       (ARRAY[15, 30, 45, 75, 112.5])[1 + t % 5], 13.8, 220,
       -21 + random() * 4, -46 + random() * 4,
       0.1 + random() * 0.3, 0.4 + random() * 1.2, 8
FROM generate_series(1, :trafos) t;

INSERT INTO consuming_units (tenant_id, transformer_id, uc_code, name, class)
SELECT :'tenant_id'::uuid, tr.id, tr.code || '-UC' || n, 'UC ' || tr.code || '/' || n,
       (ARRAY['RESIDENTIAL','COMMERCIAL','RURAL'])[1 + (n + length(tr.code)) % 3]
FROM transformers tr CROSS JOIN generate_series(1, 2) n
WHERE tr.tenant_id = :'tenant_id'::uuid AND tr.code LIKE 'TR-%';

-- leituras mensais (energia injetada no trafo e consumo das UCs)
INSERT INTO transformer_readings (tenant_id, transformer_id, reading_at, energy_kwh)
SELECT :'tenant_id'::uuid, tr.id, date_trunc('month', now()) - make_interval(months => m), 3000 + random() * 2000
FROM transformers tr CROSS JOIN generate_series(0, 5) m
WHERE tr.tenant_id = :'tenant_id'::uuid AND tr.code LIKE 'TR-%';

INSERT INTO consuming_unit_readings (tenant_id, uc_id, transformer_id, reading_at, consumption_kwh)
SELECT :'tenant_id'::uuid, uc.id, uc.transformer_id, date_trunc('month', now()) - make_interval(months => m), 1300 + random() * 900
FROM consuming_units uc CROSS JOIN generate_series(0, 5) m
WHERE uc.tenant_id = :'tenant_id'::uuid AND uc.uc_code LIKE 'TR-%';

-- balanços já calculados (mesma forma que o motor grava)
INSERT INTO transformer_balance (tenant_id, transformer_id, period_start, period_end,
                                 energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
                                 technical_loss_kwh, non_technical_loss_kwh, status, uc_count)
SELECT :'tenant_id'::uuid, tr.id, p, p + interval '1 month' - interval '1 second',
       inj, inj * (1 - pct / 100), inj * pct / 100, pct, inj * pct / 200, inj * pct / 200,
       CASE WHEN pct > 12 THEN 'CRITICAL' WHEN pct > 8 THEN 'WARNING' ELSE 'NORMAL' END, 2
FROM transformers tr
CROSS JOIN generate_series(1, 5) m
CROSS JOIN LATERAL (SELECT date_trunc('month', now()) - make_interval(months => m) AS p,
                           4000 + random() * 1000 AS inj, 2 + random() * 14 AS pct) v
WHERE tr.tenant_id = :'tenant_id'::uuid AND tr.code LIKE 'TR-%';

COMMIT;
ANALYZE substations; ANALYZE transformers; ANALYZE consuming_units;
ANALYZE transformer_readings; ANALYZE consuming_unit_readings; ANALYZE transformer_balance;
