package ingestion

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Table is a file read as headers plus string rows.
type Table struct {
	Headers   []string   `json:"headers"`
	Rows      [][]string `json:"-"`
	Format    string     `json:"format"`    // csv or xlsx
	Delimiter string     `json:"delimiter"` // ; , \t | (csv only)
	// FirstDataLine is the 1-based file line of Rows[0], for error reports.
	FirstDataLine int `json:"-"`
}

var (
	ErrEmptyFile      = errors.New("arquivo vazio")
	ErrNoHeader       = errors.New("cabeçalho não encontrado na primeira linha")
	ErrUnsupportedXLS = errors.New("formato .xls antigo não suportado; salve como .xlsx ou .csv")
)

var xlsxMagic = []byte{0x50, 0x4B, 0x03, 0x04} // ZIP container

// ReadTable reads a CSV or XLSX file. The format comes from the content
// (ZIP magic means XLSX) and falls back to the file extension.
func ReadTable(content []byte, filename string) (*Table, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, ErrEmptyFile
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if bytes.HasPrefix(content, xlsxMagic) || ext == ".xlsx" {
		return readXLSX(content)
	}
	if ext == ".xls" {
		return nil, ErrUnsupportedXLS
	}
	return readCSV(content)
}

func readCSV(content []byte) (*Table, error) {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	content = toUTF8(content)

	firstLine := content
	if i := bytes.IndexByte(content, '\n'); i >= 0 {
		firstLine = content[:i]
	}
	delim := DetectDelimiter(string(firstLine))

	r := csv.NewReader(bytes.NewReader(content))
	r.Comma = delim
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	header, err := r.Read()
	if err == io.EOF {
		return nil, ErrEmptyFile
	}
	if err != nil {
		return nil, fmt.Errorf("falha ao ler o cabeçalho: %w", err)
	}
	header = trimAll(header)
	if nonEmpty(header) == 0 {
		return nil, ErrNoHeader
	}

	t := &Table{Headers: header, Format: "csv", Delimiter: string(delim), FirstDataLine: 2}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Linha malformada vira linha vazia; o parser reporta o erro nela.
			t.Rows = append(t.Rows, []string{})
			continue
		}
		t.Rows = append(t.Rows, trimAll(rec))
	}
	return t, nil
}

func readXLSX(content []byte) (*Table, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("arquivo Excel inválido: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, ErrEmptyFile
	}
	rows, err := f.GetRows(sheets[0], excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("falha ao ler a planilha: %w", err)
	}
	// Pula linhas vazias antes do cabeçalho
	start := 0
	for start < len(rows) && nonEmpty(rows[start]) == 0 {
		start++
	}
	if start >= len(rows) {
		return nil, ErrEmptyFile
	}
	t := &Table{Headers: trimAll(rows[start]), Format: "xlsx", FirstDataLine: start + 2}
	for _, row := range rows[start+1:] {
		t.Rows = append(t.Rows, trimAll(row))
	}
	return t, nil
}

// DetectDelimiter picks the most frequent of ; , \t | outside quotes in the
// header line. Semicolon wins ties because it is the Brazilian Excel default.
func DetectDelimiter(line string) rune {
	candidates := []rune{';', ',', '\t', '|'}
	counts := map[rune]int{}
	inQuotes := false
	for _, c := range line {
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes {
			counts[c]++
		}
	}
	best, bestN := ';', 0
	for _, c := range candidates {
		if counts[c] > bestN {
			best, bestN = c, counts[c]
		}
	}
	return best
}

// toUTF8 converts Latin-1 (ISO-8859-1/Windows-1252 exports) to UTF-8 when
// the content is not valid UTF-8.
func toUTF8(b []byte) []byte {
	if isValidUTF8(b) {
		return b
	}
	var buf bytes.Buffer
	buf.Grow(len(b))
	for _, c := range b {
		buf.WriteRune(rune(c))
	}
	return buf.Bytes()
}

func isValidUTF8(b []byte) bool {
	return bytes.Equal(bytes.ToValidUTF8(b, nil), b)
}

func trimAll(row []string) []string {
	out := make([]string, len(row))
	for i, v := range row {
		out[i] = strings.TrimSpace(strings.Trim(v, "\ufeff"))
	}
	return out
}

func nonEmpty(row []string) int {
	n := 0
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			n++
		}
	}
	return n
}

// NormalizeHeader lowercases, strips accents and turns separators into "_":
// "Energia (kWh)" -> "energia_kwh", "Código Trafo" -> "codigo_trafo".
func NormalizeHeader(h string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	s, _, err := transform.String(t, h)
	if err != nil {
		s = h
	}
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastUnderscore := false
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			b.WriteRune(c)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.TrimRight(b.String(), "_")
}

// ParseNumber parses numbers in Brazilian or international notation:
// "1.234,56", "1234,56", "1,234.56", "1234.56", "1 234,56". Empty is an error.
func ParseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0, errors.New("valor vazio")
	}
	lastComma := strings.LastIndex(s, ",")
	lastDot := strings.LastIndex(s, ".")
	switch {
	case lastComma >= 0 && lastDot >= 0:
		if lastComma > lastDot { // 1.234,56
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		} else { // 1,234.56
			s = strings.ReplaceAll(s, ",", "")
		}
	case lastComma >= 0:
		if strings.Count(s, ",") > 1 { // 1,234,567
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.Replace(s, ",", ".", 1)
		}
	case strings.Count(s, ".") > 1: // 1.234.567
		s = strings.ReplaceAll(s, ".", "")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("número inválido: %q", s)
	}
	return v, nil
}

var timestampLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
	"02/01/2006 15:04:05",
	"02/01/2006 15:04",
	"02/01/2006",
	"02-01-2006 15:04:05",
	"02-01-2006",
	"2006/01/02 15:04:05",
	"2006/01/02",
	"01/2006",
	"2006-01",
}

// ParseTimestamp accepts ISO, Brazilian (dd/mm/yyyy) and month-only dates,
// plus Excel serial dates (e.g. 45292.5). Dates without zone are UTC-3
// (horario de Brasilia).
func ParseTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("data vazia")
	}
	loc := brasilia()
	for _, layout := range timestampLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	if serial, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64); err == nil && serial > 20000 && serial < 80000 {
		// Excel: dias desde 1899-12-30
		// O valor e um horario de parede: calcula em UTC e reinterpreta no fuso local.
		wall := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).
			Add(time.Duration(serial * 24 * float64(time.Hour))).Round(time.Second)
		return time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), 0, loc), nil
	}
	return time.Time{}, fmt.Errorf("data inválida: %q (use AAAA-MM-DD HH:MM:SS ou DD/MM/AAAA)", s)
}

func brasilia() *time.Location {
	if loc, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return loc
	}
	return time.FixedZone("BRT", -3*60*60)
}

// SniffLines returns up to n raw lines of a text file, for error messages.
func SniffLines(content []byte, n int) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() && len(out) < n {
		out = append(out, sc.Text())
	}
	return out
}
