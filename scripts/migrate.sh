#!/bin/sh
# Aplica as migracoes em ordem (pula *.down.sql e o 001_enable_rls_and_security.sql,
# copia identica do 001b que depende das tabelas criadas no 001_tenants_users).
# Uso: DATABASE_URL=postgres://... scripts/migrate.sh
set -eu
: "${DATABASE_URL:?defina DATABASE_URL}"
cd "$(dirname "$0")/.."
for f in \
  001_tenants_users.sql 001b_enable_rls_and_security.sql 001c_fix_tenant_rls_for_signup.sql \
  002_electrical_network.sql 003_meter_readings.sql 004_balance_results.sql \
  005_imports_alerts.sql 006_alerts_soft_delete.sql 006b_invites.sql 006c_widen_user_roles.sql \
  007_billing_fase_a.sql 008_billing_fase_b_payment.sql 009_billing_fase_c_dunning_metrics.sql \
  010_billing_fase_c_proration.sql 011_sprints_completion.sql 012_balance_loss_pct_precision.sql
do
  echo "==> $f"
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -q -f "migrations/$f"
done
