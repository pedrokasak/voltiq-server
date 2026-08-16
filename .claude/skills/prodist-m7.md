# Skill: prodist-m7
# Quando usar: qualquer tarefa que envolva cálculos de perdas ou balanço de energia

## ATENÇÃO — regra absoluta
O motor de cálculo em `internal/calc/` tem 100% de cobertura de testes.
NUNCA alterar fórmulas sem atualizar os testes correspondentes.
NUNCA alterar sem consultar o PRODIST Módulo 7 vigente (Revisão 13, 2024).

## Fórmulas implementadas

### Balanço de energia (Seção 6.1)
```
Balanço (kWh) = Energia_Injetada - Σ(Consumo_UCs)
% Perda Total = (Balanço / Energia_Injetada) × 100
```

### Perdas técnicas no transformador (Seção 6.2)
```
PT_trafo = P0 × T + Pcc × (Ic/In)² × T

Onde:
P0  = perdas no núcleo em vazio (kW) — dado de placa, ocorre 24h/dia
Pcc = perdas nos enrolamentos com carga nominal (kW) — dado de placa
Ic  = corrente média do período (A) — medida ou estimada
In  = corrente nominal (A) = (kVA × 1000) / (√3 × V_secundario)
T   = duração do período em horas (ex: 720h para 30 dias)

Resultado em kWh.
```

### Corrente nominal trifásico
```
In = (kVA × 1000) / (√3 × V_secundario)
√3 ≈ 1.7320508
```

### Perda não técnica (Seção 6.4)
```
PNT = Perda_Total - PT_trafo
Se PNT < 0 → reportar 0 (dado de placa inconsistente, não PNT negativa)
```

### Classificação de status
```
NORMAL  → % perda < 80% do limite ANEEL configurado
ATENCAO → % perda entre 80% e 100% do limite
CRITICO → % perda ≥ limite ANEEL configurado
```

## Arquivos relevantes
```
internal/calc/
  prodist_m7.go       ← implementação das fórmulas
  prodist_m7_test.go  ← testes com valores de referência documentados
  balance.go          ← cálculo de balanço geral
```

## Casos de borda documentados

**Sem corrente medida (Ic = 0):**
→ Usar estimativa conservadora de 50% de carregamento
→ Documentar este fallback no relatório gerado
→ `perdaEnrolamento = Pcc × 0.5² × T`

**PNT negativa:**
→ Indica que perdas técnicas calculadas excedem o balanço medido
→ Causa: dado de placa inconsistente ou erro de medição
→ Reportar PNT = 0, não valor negativo
→ Logar aviso para investigação

**Sem UCs vinculadas:**
→ Toda energia injetada = perda (possível trafo não cadastrado corretamente)
→ Status automático = CRITICO

**Energia injetada = 0:**
→ Retornar erro `ErrEnergiaInjetadaZero`
→ Não calcular (divisão por zero)

## Valores típicos de P0 e Pcc por potência

| kVA | P0 típico (kW) | Pcc típico (kW) |
|---|---|---|
| 15  | 0.095 | 0.560 |
| 30  | 0.140 | 0.890 |
| 45  | 0.180 | 0.890 |
| 75  | 0.265 | 1.350 |
| 112 | 0.320 | 1.750 |
| 150 | 0.420 | 2.100 |
| 300 | 0.700 | 3.900 |
| 500 | 1.050 | 6.200 |

Valores de referência — sempre preferir dados reais da placa do equipamento.

## Ao adicionar novo cálculo PRODIST

1. Criar função em `internal/calc/` com nome descritivo
2. Adicionar comentário com referência à seção do PRODIST
3. Criar teste com valores de referência conhecidos
4. Documentar casos de borda esperados
5. Nunca retornar valores negativos para perda ou energia
