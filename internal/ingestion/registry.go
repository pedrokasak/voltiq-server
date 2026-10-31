package ingestion

import (
	"strconv"
	"strings"

	"github.com/voltiq/server/internal/domain"
)

// TransformerRow is one parsed line of a transformer registry file.
type TransformerRow struct {
	Line           int
	Transformer    domain.Transformer
	SubstationCode string
}

// UCRow is one parsed line of a consuming unit registry file.
type UCRow struct {
	Line           int
	UCCode         string
	TransformerRef string
	Name           string
	Class          *domain.UCClass
}

// ucClassAliases maps the classes written in Portuguese or English to the
// domain enum.
var ucClassAliases = map[string]domain.UCClass{
	"residencial": domain.UCClassResidential, "residential": domain.UCClassResidential, "b1": domain.UCClassResidential,
	"comercial": domain.UCClassCommercial, "commercial": domain.UCClassCommercial, "b3": domain.UCClassCommercial,
	"industrial": domain.UCClassIndustrial,
	"rural":      domain.UCClassRural, "b2": domain.UCClassRural,
	"poder_publico": domain.UCClassPublicPower, "public_power": domain.UCClassPublicPower, "publico": domain.UCClassPublicPower,
}

// ParseUCClass converts a class written in the file to the domain enum.
func ParseUCClass(raw string) (*domain.UCClass, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	c, ok := ucClassAliases[NormalizeHeader(raw)]
	if !ok {
		return nil, false
	}
	return &c, true
}

// ParseTransformerRegistry converts the rows of a registry table, applying
// domain.ValidateTransformer to each one.
func ParseTransformerRegistry(table *Table, explicit map[string]string) ([]TransformerRow, []ParseError, MappingReport, error) {
	schema, _ := SchemaFor(KindTransformerRegistry)
	m, report := schema.ResolveMapping(table.Headers, explicit)
	if err := report.MissingError(); err != nil {
		return nil, nil, report, err
	}

	var rows []TransformerRow
	var errs []ParseError
	seen := map[string]int{}

	for i, raw := range table.Rows {
		line := table.FirstDataLine + i
		if nonEmpty(raw) == 0 {
			continue
		}
		t := domain.Transformer{Code: strings.TrimSpace(m.Cell(raw, "code")), Active: true}

		var rowErr *ParseError
		num := func(field string, required bool) *float64 {
			v := m.Cell(raw, field)
			if v == "" {
				if required && rowErr == nil {
					rowErr = &ParseError{Line: line, Column: field, Message: "valor obrigatório"}
				}
				return nil
			}
			f, err := ParseNumber(v)
			if err != nil {
				if rowErr == nil {
					rowErr = &ParseError{Line: line, Column: field, Value: v, Message: err.Error()}
				}
				return nil
			}
			return &f
		}

		if p := num("power_kva", true); p != nil {
			t.PowerKVA = *p
		}
		if v := num("primary_voltage_kv", false); v != nil {
			t.PrimaryVoltageKV = *v
		}
		if v := num("secondary_voltage_v", false); v != nil {
			t.SecondaryVoltageV = *v
		}
		t.Lat = num("lat", false)
		t.Lng = num("lng", false)
		t.CoreLossKW = num("core_loss_kw", false)
		t.WindingLossKW = num("winding_loss_kw", false)
		t.LossLimitPct = num("loss_limit_pct", false)

		if rowErr != nil {
			errs = append(errs, *rowErr)
			continue
		}
		if err := domain.ValidateTransformer(t); err != nil {
			if vErr, ok := err.(*domain.ValidationError); ok && len(vErr.Fields) > 0 {
				errs = append(errs, ParseError{Line: line, Column: vErr.Fields[0].Field, Message: vErr.Fields[0].Message})
				continue
			}
		}
		if prev, dup := seen[strings.ToUpper(t.Code)]; dup {
			errs = append(errs, ParseError{Line: line, Column: "code", Value: t.Code,
				Message: "código repetido no arquivo (primeira ocorrência na linha " + strconv.Itoa(prev) + ")"})
			continue
		}
		seen[strings.ToUpper(t.Code)] = line

		rows = append(rows, TransformerRow{Line: line, Transformer: t, SubstationCode: strings.TrimSpace(m.Cell(raw, "substation"))})
	}
	return rows, errs, report, nil
}

// ParseUCRegistry converts the rows of a consuming unit registry table.
func ParseUCRegistry(table *Table, explicit map[string]string) ([]UCRow, []ParseError, MappingReport, error) {
	schema, _ := SchemaFor(KindUCRegistry)
	m, report := schema.ResolveMapping(table.Headers, explicit)
	if err := report.MissingError(); err != nil {
		return nil, nil, report, err
	}

	var rows []UCRow
	var errs []ParseError
	seen := map[string]int{}

	for i, raw := range table.Rows {
		line := table.FirstDataLine + i
		if nonEmpty(raw) == 0 {
			continue
		}
		code := strings.TrimSpace(m.Cell(raw, "uc_code"))
		ref := strings.TrimSpace(m.Cell(raw, "transformer"))
		switch {
		case code == "":
			errs = append(errs, ParseError{Line: line, Column: "uc_code", Message: "código da UC vazio"})
			continue
		case ref == "":
			errs = append(errs, ParseError{Line: line, Column: "transformer", Message: "transformador vazio"})
			continue
		}
		class, ok := ParseUCClass(m.Cell(raw, "class"))
		if !ok {
			errs = append(errs, ParseError{Line: line, Column: "class", Value: m.Cell(raw, "class"),
				Message: "classe deve ser RESIDENCIAL, COMERCIAL, INDUSTRIAL, RURAL ou PODER_PUBLICO"})
			continue
		}
		if prev, dup := seen[strings.ToUpper(code)]; dup {
			errs = append(errs, ParseError{Line: line, Column: "uc_code", Value: code,
				Message: "código repetido no arquivo (primeira ocorrência na linha " + strconv.Itoa(prev) + ")"})
			continue
		}
		seen[strings.ToUpper(code)] = line
		rows = append(rows, UCRow{Line: line, UCCode: code, TransformerRef: ref, Name: strings.TrimSpace(m.Cell(raw, "name")), Class: class})
	}
	return rows, errs, report, nil
}
