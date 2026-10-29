#!/bin/sh
# Aplica as migracoes em ordem (pula *.down.sql e o 001_enable_rls_and_security.sql,
# copia identica do 001b que depende das tabelas criadas no 001_tenants_users).
# O 001b ainda referencia as tabelas legadas em portugues (subestacoes, trafos,
# leituras_*...), que nao existem num banco novo; ele roda sem ON_ERROR_STOP para
# aplicar o que vale (extensoes, RLS de tenants/users/invites, audit_log e sua
# funcao) e ignorar os comandos das tabelas legadas. As politicas das tabelas
# atuais vem das migracoes 002+ e da 011. O 003 chama add_compression_policy sem
# ativar timescaledb.compress na tabela, o que o TimescaleDB recusa num banco
# novo; as politicas de compressao rodam a parte e uma falha nelas so gera aviso.
# As migracoes 001-010 nao sao editadas.
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
  if [ "$f" = 001b_enable_rls_and_security.sql ]; then
    psql "$DATABASE_URL" -v ON_ERROR_STOP=0 -q -f "migrations/$f" 2>&1 \
      | grep -v 'does not exist' | grep -v 'current transaction is aborted' || true
  elif [ "$f" = 003_meter_readings.sql ]; then
    grep -v '^SELECT add_compression_policy' "migrations/$f" \
      | psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -q
    grep '^SELECT add_compression_policy' "migrations/$f" \
      | psql "$DATABASE_URL" -v ON_ERROR_STOP=0 -q 2>&1 \
      | sed 's/^/aviso (compressao nao configurada): /' || true
  else
    psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -q -f "migrations/$f"
  fi
done
