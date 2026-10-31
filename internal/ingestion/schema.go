package ingestion

import (
	"fmt"
	"sort"
	"strings"
)

// Field describes a column the importer understands.
type Field struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Required    bool     `json:"required"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
	Example     string   `json:"example"`
}

// Schema is the set of columns of one kind of file.
type Schema struct {
	Kind   string  `json:"kind"`
	Title  string  `json:"title"`
	Fields []Field `json:"fields"`
}

// Mapping maps a schema field name to a column index of the table.
type Mapping map[string]int

// MappingReport explains how headers were matched to fields.
type MappingReport struct {
	Columns         map[string]string `json:"columns"`          // field -> header
	MissingRequired []string          `json:"missing_required"` // required fields without a column
	UnmappedHeaders []string          `json:"unmapped_headers"` // headers not used
}

// Tipos de arquivo aceitos pela importacao.
const (
	KindTransformerReadings = "transformer_readings"
	KindUCReadings          = "uc_readings"
	KindTransformerRegistry = "transformers"
	KindUCRegistry          = "consuming_units"
)

var readingAtField = Field{
	Name: "reading_at", Label: "Data/hora da leitura", Required: true,
	Aliases:     []string{"reading_at", "leitura_em", "data_leitura", "data_hora", "data", "date", "datetime", "timestamp", "dt_leitura", "periodo", "mes_referencia", "referencia"},
	Description: "AAAA-MM-DD HH:MM:SS, DD/MM/AAAA ou data do Excel",
	Example:     "2025-01-31 23:59:59",
}

// Schemas returns every import schema indexed by kind.
func Schemas() map[string]Schema {
	return map[string]Schema{
		KindTransformerReadings: {
			Kind: KindTransformerReadings, Title: "Leituras de transformador (medidor primário)",
			Fields: []Field{
				{Name: "transformer", Label: "Transformador", Required: true,
					Aliases:     []string{"transformer_id", "transformer_code", "transformer", "trafo_id", "trafo", "codigo_trafo", "cod_trafo", "transformador", "codigo_transformador", "id_trafo"},
					Description: "Código ou ID do transformador", Example: "TRF-001"},
				readingAtField,
				{Name: "energy_kwh", Label: "Energia injetada (kWh)", Required: true,
					Aliases:     []string{"energy_kwh", "energia_kwh", "energia_injetada_kwh", "energia_injetada", "energia", "kwh", "energy", "consumo_kwh"},
					Description: "Energia medida no período (kWh)", Example: "15000,50"},
				{Name: "demand_kw", Label: "Demanda (kW)",
					Aliases:     []string{"demand_kw", "demanda_kw", "demanda", "demand", "kw"},
					Description: "Demanda média medida (kW)", Example: "120,5"},
				{Name: "power_factor", Label: "Fator de potência",
					Aliases:     []string{"power_factor", "fator_potencia", "fator_de_potencia", "fp", "pf", "cos_phi"},
					Description: "Entre 0 e 1", Example: "0,92"},
			},
		},
		KindUCReadings: {
			Kind: KindUCReadings, Title: "Leituras de unidades consumidoras",
			Fields: []Field{
				{Name: "uc", Label: "Unidade consumidora", Required: true,
					Aliases:     []string{"uc_id", "uc_code", "uc", "codigo_uc", "cod_uc", "unidade_consumidora", "instalacao", "numero_instalacao", "codigo_unidade"},
					Description: "Código ou ID da UC", Example: "UC-1001"},
				readingAtField,
				{Name: "consumption_kwh", Label: "Consumo (kWh)", Required: true,
					Aliases:     []string{"consumption_kwh", "consumo_kwh", "consumo", "consumption", "kwh", "energia_kwh", "energy_kwh", "energia_faturada_kwh"},
					Description: "Consumo faturado no período (kWh)", Example: "250,75"},
			},
		},
		KindTransformerRegistry: {
			Kind: KindTransformerRegistry, Title: "Cadastro de transformadores",
			Fields: []Field{
				{Name: "code", Label: "Código", Required: true,
					Aliases: []string{"code", "codigo", "transformer_code", "codigo_trafo", "cod_trafo", "trafo", "transformador", "codigo_transformador"},
					Example: "TRF-001"},
				{Name: "power_kva", Label: "Potência (kVA)", Required: true,
					Aliases: []string{"power_kva", "potencia_kva", "potencia", "kva", "potencia_nominal", "potencia_nominal_kva"},
					Example: "112,5"},
				{Name: "primary_voltage_kv", Label: "Tensão primária (kV)",
					Aliases: []string{"primary_voltage_kv", "tensao_primaria_kv", "tensao_primaria", "vp_kv", "tensao_mt_kv"},
					Example: "13,8"},
				{Name: "secondary_voltage_v", Label: "Tensão secundária (V)",
					Aliases: []string{"secondary_voltage_v", "tensao_secundaria_v", "tensao_secundaria", "vs_v", "tensao_bt_v"},
					Example: "220"},
				{Name: "lat", Label: "Latitude", Aliases: []string{"lat", "latitude", "y"}, Example: "-23,550520"},
				{Name: "lng", Label: "Longitude", Aliases: []string{"lng", "lon", "long", "longitude", "x"}, Example: "-46,633308"},
				{Name: "core_loss_kw", Label: "Perda no núcleo P0 (kW)",
					Aliases: []string{"core_loss_kw", "perda_nucleo_kw", "perda_nucleo", "p0", "p0_kw", "perda_vazio_kw", "perdas_em_vazio_kw"},
					Example: "0,340"},
				{Name: "winding_loss_kw", Label: "Perda nos enrolamentos Pcc (kW)",
					Aliases: []string{"winding_loss_kw", "perda_enrolamento_kw", "perda_enrolamentos_kw", "perda_enrolamento", "pcc", "pcc_kw", "perda_carga_kw", "perdas_em_carga_kw"},
					Example: "1,450"},
				{Name: "loss_limit_pct", Label: "Limite de perdas ANEEL (%)",
					Aliases: []string{"loss_limit_pct", "limite_perda_pct", "limite_perdas", "limite_perda", "limite", "limite_aneel"},
					Example: "10"},
				{Name: "substation", Label: "Subestação",
					Aliases:     []string{"substation", "substation_code", "subestacao", "codigo_subestacao", "cod_subestacao", "se"},
					Description: "Código da subestação (criada se não existir)", Example: "SE-CENTRO"},
			},
		},
		KindUCRegistry: {
			Kind: KindUCRegistry, Title: "Cadastro de unidades consumidoras",
			Fields: []Field{
				{Name: "uc_code", Label: "Código da UC", Required: true,
					Aliases: []string{"uc_code", "codigo_uc", "cod_uc", "uc", "unidade_consumidora", "instalacao", "numero_instalacao", "codigo"},
					Example: "UC-1001"},
				{Name: "transformer", Label: "Transformador", Required: true,
					Aliases: []string{"transformer", "transformer_code", "transformer_id", "codigo_trafo", "cod_trafo", "trafo", "transformador"},
					Example: "TRF-001"},
				{Name: "name", Label: "Nome", Aliases: []string{"name", "nome", "cliente", "titular", "consumidor"}, Example: "Sítio Boa Vista"},
				{Name: "class", Label: "Classe",
					Aliases:     []string{"class", "classe", "classe_consumo", "categoria"},
					Description: "RESIDENCIAL, COMERCIAL, INDUSTRIAL, RURAL ou PODER_PUBLICO", Example: "RURAL"},
			},
		},
	}
}

// SchemaFor returns the schema of a kind.
func SchemaFor(kind string) (Schema, bool) {
	s, ok := Schemas()[kind]
	return s, ok
}

// ResolveMapping matches table headers to schema fields. explicit maps a
// field name to the header the user chose; the remaining fields are matched
// by normalized name and aliases.
func (s Schema) ResolveMapping(headers []string, explicit map[string]string) (Mapping, MappingReport) {
	normalized := make([]string, len(headers))
	for i, h := range headers {
		normalized[i] = NormalizeHeader(h)
	}

	m := Mapping{}
	used := map[int]bool{}
	report := MappingReport{Columns: map[string]string{}}

	for field, header := range explicit {
		if header == "" || !s.hasField(field) {
			continue
		}
		want := NormalizeHeader(header)
		for i, h := range normalized {
			if h == want && !used[i] {
				m[field] = i
				used[i] = true
				report.Columns[field] = headers[i]
				break
			}
		}
	}

	for _, f := range s.Fields {
		if _, done := m[f.Name]; done {
			continue
		}
		candidates := append([]string{f.Name}, f.Aliases...)
		for _, alias := range candidates {
			idx := -1
			for i, h := range normalized {
				if !used[i] && h == alias {
					idx = i
					break
				}
			}
			if idx >= 0 {
				m[f.Name] = idx
				used[idx] = true
				report.Columns[f.Name] = headers[idx]
				break
			}
		}
	}

	for _, f := range s.Fields {
		if _, ok := m[f.Name]; !ok && f.Required {
			report.MissingRequired = append(report.MissingRequired, f.Name)
		}
	}
	for i, h := range headers {
		if !used[i] && strings.TrimSpace(h) != "" {
			report.UnmappedHeaders = append(report.UnmappedHeaders, h)
		}
	}
	sort.Strings(report.MissingRequired)
	return m, report
}

func (s Schema) hasField(name string) bool {
	for _, f := range s.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

// Template returns a semicolon CSV with the header and example rows of the
// schema (RF08.3: CSV de exemplo).
func (s Schema) Template(examples [][]string) []byte {
	var b strings.Builder
	b.WriteString("\ufeff") // BOM: Excel abre acentos corretamente
	names := make([]string, len(s.Fields))
	for i, f := range s.Fields {
		names[i] = f.Name
	}
	b.WriteString(strings.Join(names, ";"))
	b.WriteString("\r\n")
	for _, row := range examples {
		b.WriteString(strings.Join(row, ";"))
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

// Cell returns the value of field in row, or "" when unmapped.
func (m Mapping) Cell(row []string, field string) string {
	i, ok := m[field]
	if !ok || i >= len(row) {
		return ""
	}
	return row[i]
}

// MissingError describes the required columns not found in a file.
func (r MappingReport) MissingError() error {
	if len(r.MissingRequired) == 0 {
		return nil
	}
	return fmt.Errorf("colunas obrigatórias não encontradas: %s", strings.Join(r.MissingRequired, ", "))
}
