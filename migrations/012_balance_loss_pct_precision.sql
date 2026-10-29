-- Migracao 012: amplia transformer_balance.loss_pct.
-- DECIMAL(6,4) (migracao 004) aceita no maximo 99,9999%: um balanco com
-- perda de 100% ou com consumo acima do dobro do injetado (dado inconsistente)
-- falhava ao gravar. DECIMAL(9,4) aceita ate +-99.999,9999%.
ALTER TABLE transformer_balance ALTER COLUMN loss_pct TYPE DECIMAL(9,4);
