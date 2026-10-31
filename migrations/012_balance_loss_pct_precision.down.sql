-- Reversao da 012 (falha se houver valores fora de DECIMAL(6,4)).
ALTER TABLE transformer_balance ALTER COLUMN loss_pct TYPE DECIMAL(6,4);
