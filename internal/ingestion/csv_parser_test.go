package ingestion_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/ingestion"
)

func brt(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*60*60)
	}
	return loc
}

func TestCSVParser_ParseTransformerReadings(t *testing.T) {
	parser := ingestion.NewCSVParser()

	csvData := `transformer_id;reading_at;energy_kwh;demand_kw;power_factor
	trafo-001;2024-01-01 00:00:00;1500.50;120.5;0.92
	trafo-001;2024-01-02 00:00:00;1600.75;125.3;0.91
	trafo-002;2024-01-01 00:00:00;2000.00;150.0;0.95`

	result, err := parser.Parse(strings.NewReader(csvData), ingestion.ReadingTypeTransformer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Success) != 3 {
		t.Fatalf("expected 3 successful parses, got %d (errors: %v)", len(result.Success), result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
	if result.Success[0].TransformerID != "trafo-001" {
		t.Errorf("expected transformer_id trafo-001, got %s", result.Success[0].TransformerID)
	}
	if result.Success[0].EnergyKWh != 1500.50 {
		t.Errorf("expected energy_kwh 1500.50, got %f", result.Success[0].EnergyKWh)
	}
	if result.Success[0].Line != 2 {
		t.Errorf("expected first data line 2, got %d", result.Success[0].Line)
	}
}

func TestCSVParser_ParseUCReadings(t *testing.T) {
	parser := ingestion.NewCSVParser()

	csvData := `uc_id;reading_at;consumption_kwh
uc-001;2024-01-01 00:00:00;250.75
uc-001;2024-01-02 00:00:00;280.50
uc-002;2024-01-01 00:00:00;350.00`

	result, err := parser.Parse(strings.NewReader(csvData), ingestion.ReadingTypeUC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Success) != 3 {
		t.Fatalf("expected 3 successful parses, got %d", len(result.Success))
	}
	if result.Success[0].UCID != "uc-001" {
		t.Errorf("expected uc_id uc-001, got %s", result.Success[0].UCID)
	}
	if result.Success[0].EnergyKWh != 250.75 {
		t.Errorf("expected consumption_kwh 250.75, got %f", result.Success[0].EnergyKWh)
	}
}

func TestCSVParser_ParseErrors(t *testing.T) {
	parser := ingestion.NewCSVParser()

	csvData := `transformer_id;reading_at;energy_kwh
trafo-001;2024-01-01 00:00:00;1500.50
;2024-01-02 00:00:00;1600.75
trafo-002;invalid-date;2000.00
trafo-003;2024-01-01 00:00:00;not-a-number
trafo-004;2024-01-01 00:00:00;-10`

	result, err := parser.Parse(strings.NewReader(csvData), ingestion.ReadingTypeTransformer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Success) != 1 {
		t.Errorf("expected 1 successful parse, got %d", len(result.Success))
	}
	if len(result.Errors) != 4 {
		t.Fatalf("expected 4 errors, got %d: %v", len(result.Errors), result.Errors)
	}
	wantLines := []int{3, 4, 5, 6}
	for i, e := range result.Errors {
		if e.Line != wantLines[i] {
			t.Errorf("error %d: expected line %d, got %d", i, wantLines[i], e.Line)
		}
	}
	if result.Errors[3].Column != "energy_kwh" || !strings.Contains(result.Errors[3].Message, "negativo") {
		t.Errorf("expected negative value error on energy_kwh, got %+v", result.Errors[3])
	}
}

func TestCSVParser_MissingRequiredColumn(t *testing.T) {
	parser := ingestion.NewCSVParser()
	_, err := parser.Parse(strings.NewReader("transformer_id;energy_kwh\nT1;10"), ingestion.ReadingTypeTransformer)
	if err == nil || !strings.Contains(err.Error(), "reading_at") {
		t.Fatalf("expected missing reading_at error, got %v", err)
	}
}

func TestCSVParser_ParseTimestamp(t *testing.T) {
	parser := ingestion.NewCSVParser()
	loc := brt(t)

	tests := []struct {
		name     string
		format   string
		expected time.Time
	}{
		{"ISO", "2024-01-01 15:04:05", time.Date(2024, 1, 1, 15, 4, 5, 0, loc)},
		{"ISO_T", "2024-01-01T15:04:05", time.Date(2024, 1, 1, 15, 4, 5, 0, loc)},
		{"ISO_zone", "2024-01-01T15:04:05Z", time.Date(2024, 1, 1, 15, 4, 5, 0, time.UTC)},
		{"BR", "01/01/2024 15:04:05", time.Date(2024, 1, 1, 15, 4, 5, 0, loc)},
		{"BR_date", "01/01/2024", time.Date(2024, 1, 1, 0, 0, 0, 0, loc)},
		{"month", "01/2024", time.Date(2024, 1, 1, 0, 0, 0, 0, loc)},
		{"excel_serial", "45292", time.Date(2024, 1, 1, 0, 0, 0, 0, loc)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csvData := "transformer_id;reading_at;energy_kwh\ntrafo-001;" + tt.format + ";100.0"
			result, err := parser.Parse(strings.NewReader(csvData), ingestion.ReadingTypeTransformer)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Success) != 1 {
				t.Fatalf("expected 1 successful parse, got %d (%v)", len(result.Success), result.Errors)
			}
			if !result.Success[0].ReadingAt.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result.Success[0].ReadingAt)
			}
		})
	}
}

func TestCSVParser_DynamicFormat(t *testing.T) {
	parser := ingestion.NewCSVParser()

	// Cabecalhos em portugues, virgula decimal, separador ponto e virgula,
	// fator de potencia em percentual e BOM do Excel.
	csvData := "\ufeffCódigo Trafo;Data Leitura;Energia (kWh);Demanda;FP\n" +
		"TRF-001;31/01/2025;1.234,56;120,5;92\n"

	result, err := parser.Parse(strings.NewReader(csvData), ingestion.ReadingTypeTransformer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Success) != 1 {
		t.Fatalf("expected 1 reading, got %d (%v)", len(result.Success), result.Errors)
	}
	r := result.Success[0]
	if r.TransformerID != "TRF-001" || r.EnergyKWh != 1234.56 {
		t.Errorf("unexpected reading %+v", r)
	}
	if r.DemandKW == nil || *r.DemandKW != 120.5 {
		t.Errorf("expected demand 120.5, got %v", r.DemandKW)
	}
	if r.PowerFactor == nil || *r.PowerFactor != 0.92 {
		t.Errorf("expected power factor 0.92, got %v", r.PowerFactor)
	}
	if result.Mapping.Columns["energy_kwh"] != "Energia (kWh)" {
		t.Errorf("expected energy column mapped, got %v", result.Mapping.Columns)
	}
}

func TestCSVParser_CommaDelimiterAndExplicitMapping(t *testing.T) {
	parser := ingestion.NewCSVParser()
	csvData := "medidor,quando,valor\nT1,2025-01-01,10.5\n"

	if _, err := parser.ParseFile([]byte(csvData), "f.csv", ingestion.ReadingTypeTransformer, nil); err == nil {
		t.Fatal("expected missing columns without mapping")
	}

	result, err := parser.ParseFile([]byte(csvData), "f.csv", ingestion.ReadingTypeTransformer, map[string]string{
		"transformer": "medidor", "reading_at": "quando", "energy_kwh": "valor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Table.Delimiter != "," {
		t.Errorf("expected comma delimiter, got %q", result.Table.Delimiter)
	}
	if len(result.Success) != 1 || result.Success[0].EnergyKWh != 10.5 {
		t.Fatalf("unexpected result %+v %v", result.Success, result.Errors)
	}
}

func TestReadTable_XLSX(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetRow("Sheet1", "A1", &[]any{"uc", "data", "consumo"})
	_ = f.SetSheetRow("Sheet1", "A2", &[]any{"UC-1", "2025-01-31", 250.5})
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}

	result, err := ingestion.NewCSVParser().ParseFile(buf.Bytes(), "leituras.xlsx", ingestion.ReadingTypeUC, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Table.Format != "xlsx" {
		t.Errorf("expected xlsx format, got %s", result.Table.Format)
	}
	if len(result.Success) != 1 || result.Success[0].UCID != "UC-1" || result.Success[0].EnergyKWh != 250.5 {
		t.Fatalf("unexpected result %+v %v", result.Success, result.Errors)
	}
}

func TestReadTable_Errors(t *testing.T) {
	if _, err := ingestion.ReadTable([]byte("  "), "a.csv"); err != ingestion.ErrEmptyFile {
		t.Errorf("expected ErrEmptyFile, got %v", err)
	}
	if _, err := ingestion.ReadTable([]byte("abc"), "a.xls"); err != ingestion.ErrUnsupportedXLS {
		t.Errorf("expected ErrUnsupportedXLS, got %v", err)
	}
	// Latin-1 vira UTF-8
	tbl, err := ingestion.ReadTable([]byte("c\xf3digo;pot\xeancia\nT1;10\n"), "a.csv")
	if err != nil {
		t.Fatal(err)
	}
	if tbl.Headers[0] != "código" || tbl.Headers[1] != "potência" {
		t.Errorf("unexpected headers %q", tbl.Headers)
	}
}

func TestParseNumber(t *testing.T) {
	cases := map[string]float64{
		"1500.50": 1500.5, "1500,50": 1500.5, "1.234,56": 1234.56, "1,234.56": 1234.56,
		"1.234.567": 1234567, "1,234,567": 1234567, " 1 234,5 ": 1234.5, "-3,2": -3.2, "0": 0,
	}
	for in, want := range cases {
		got, err := ingestion.ParseNumber(in)
		if err != nil || got != want {
			t.Errorf("ParseNumber(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "1,2,3.4.5", "NaN"} {
		if _, err := ingestion.ParseNumber(bad); err == nil {
			t.Errorf("ParseNumber(%q) expected error", bad)
		}
	}
}

func TestDetectDelimiter(t *testing.T) {
	cases := map[string]rune{
		"a;b;c": ';', "a,b,c": ',', "a\tb\tc": '\t', "a|b|c": '|', `"a,b";c;d`: ';', "single": ';',
	}
	for line, want := range cases {
		if got := ingestion.DetectDelimiter(line); got != want {
			t.Errorf("DetectDelimiter(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestNormalizeHeader(t *testing.T) {
	cases := map[string]string{
		"Energia (kWh)": "energia_kwh", "Código Trafo": "codigo_trafo", "  Data/Hora ": "data_hora", "P0_kW": "p0_kw",
	}
	for in, want := range cases {
		if got := ingestion.NormalizeHeader(in); got != want {
			t.Errorf("NormalizeHeader(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTransformerRegistry(t *testing.T) {
	csvData := "codigo;potencia_kva;tensao_primaria_kv;tensao_secundaria_v;latitude;longitude;p0;pcc;limite;subestacao\n" +
		"TRF-001;112,5;13,8;220;-23,55;-46,63;0,34;1,45;10;SE-01\n" +
		"TRF-002;-5;13,8;220;;;;;;\n" +
		"TRF-001;75;13,8;220;;;;;;\n" +
		"TRF-003;75;13,8;220;-23,5;;;;;\n" +
		"TRF-004;abc;13,8;220;;;;;;\n"
	tbl, err := ingestion.ReadTable([]byte(csvData), "t.csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, errs, _, err := ingestion.ParseTransformerRegistry(tbl, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 valid row, got %d (%v)", len(rows), errs)
	}
	r := rows[0]
	if r.Transformer.Code != "TRF-001" || r.Transformer.PowerKVA != 112.5 || r.SubstationCode != "SE-01" {
		t.Errorf("unexpected row %+v", r)
	}
	if r.Transformer.CoreLossKW == nil || *r.Transformer.CoreLossKW != 0.34 {
		t.Errorf("expected P0 0.34, got %v", r.Transformer.CoreLossKW)
	}
	if len(errs) != 4 {
		t.Fatalf("expected 4 errors, got %d: %v", len(errs), errs)
	}
	if errs[0].Column != "power_kva" || errs[1].Column != "code" || errs[2].Column != "lat" || errs[3].Column != "power_kva" {
		t.Errorf("unexpected error columns: %+v", errs)
	}
}

func TestParseUCRegistry(t *testing.T) {
	csvData := "codigo_uc;trafo;nome;classe\nUC-1;TRF-001;Sitio;Rural\nUC-2;TRF-001;;Poder Público\nUC-3;;x;rural\nUC-4;TRF-001;;marte\nUC-1;TRF-002;;\n"
	tbl, err := ingestion.ReadTable([]byte(csvData), "u.csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, errs, _, err := ingestion.ParseUCRegistry(tbl, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(errs) != 3 {
		t.Fatalf("expected 2 rows and 3 errors, got %d/%d: %v", len(rows), len(errs), errs)
	}
	if rows[0].Class == nil || *rows[0].Class != domain.UCClassRural {
		t.Errorf("expected RURAL, got %v", rows[0].Class)
	}
	if rows[1].Class == nil || *rows[1].Class != domain.UCClassPublicPower {
		t.Errorf("expected PUBLIC_POWER, got %v", rows[1].Class)
	}
}

func TestSchemaTemplate(t *testing.T) {
	s, ok := ingestion.SchemaFor(ingestion.KindTransformerReadings)
	if !ok {
		t.Fatal("schema not found")
	}
	tpl := string(s.Template([][]string{{"TRF-001", "2025-01-31 23:59:59", "15000,5", "", ""}}))
	if !strings.HasPrefix(tpl, "\ufefftransformer;reading_at;energy_kwh;demand_kw;power_factor\r\n") {
		t.Errorf("unexpected template header: %q", tpl)
	}
	result, err := ingestion.NewCSVParser().ParseFile([]byte(tpl), "tpl.csv", ingestion.ReadingTypeTransformer, nil)
	if err != nil || len(result.Success) != 1 {
		t.Fatalf("template must parse back: %v %v", err, result)
	}
}
