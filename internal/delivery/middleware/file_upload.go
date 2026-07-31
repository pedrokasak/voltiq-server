package middleware

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/voltiq/server/internal/delivery/request"
)

var (
	ErrInvalidFileType = errors.New("invalid file type")
	ErrMalformedCSV    = errors.New("malformed CSV file")
	ErrFileTooLarge    = errors.New("file too large")
)

// FileUploadConfig holds file upload validation configuration
type FileUploadConfig struct {
	AllowedMimeTypes  []string
	AllowedMagicBytes map[string][][]byte
	MaxFileSize       int64
	ValidateCSV       bool
}

// FileUploadMiddleware validates file uploads
type FileUploadMiddleware struct {
	config FileUploadConfig
}

// Magic bytes signatures for CSV files
var csvMagicBytes = [][]byte{
	[]byte{0xEF, 0xBB, 0xBF}, // UTF-8 BOM
}

// Text-based signatures
var textSignatures = []string{
	"transformer_id",
	"uc_id",
	"reading_at",
	"energy_kwh",
	"consumption_kwh",
}

// NewFileUploadMiddleware creates a new FileUploadMiddleware
func NewFileUploadMiddleware(config FileUploadConfig) *FileUploadMiddleware {
	if config.AllowedMimeTypes == nil {
		config.AllowedMimeTypes = []string{
			"text/csv",
			"text/plain",
			"application/vnd.ms-excel",
		}
	}

	if config.AllowedMagicBytes == nil {
		config.AllowedMagicBytes = make(map[string][][]byte)
	}

	if config.MaxFileSize == 0 {
		config.MaxFileSize = 32 << 20 // 32MB default
	}

	return &FileUploadMiddleware{
		config: config,
	}
}

// ValidateFile validates uploaded file
func (m *FileUploadMiddleware) ValidateFile(w http.ResponseWriter, r *http.Request, file io.Reader) bool {
	// Limit reader to max file size
	limitedReader := io.LimitReader(file, m.config.MaxFileSize+1)

	// Read first bytes for magic number detection
	buf := make([]byte, 512)
	n, err := limitedReader.Read(buf)
	if err != nil && err != io.EOF {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail(
			"FILE_READ_ERROR",
			"Failed to read file",
			nil,
		))
		return false
	}

	buf = buf[:n]

	// Check file size
	if int64(n) == m.config.MaxFileSize+1 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail(
			"FILE_TOO_LARGE",
			"File exceeds maximum size of 32MB",
			nil,
		))
		return false
	}

	// Validate MIME type
	contentType := http.DetectContentType(buf)
	if !isAllowedMime(m.config.AllowedMimeTypes, contentType) {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail(
			"INVALID_FILE_TYPE",
			"File type not allowed. Allowed types: CSV",
			map[string]any{
				"detected_type": contentType,
				"allowed_types": m.config.AllowedMimeTypes,
			},
		))
		return false
	}

	// Validate magic bytes for CSV
	if !m.hasValidMagicBytes(buf) {
		// For CSV, also check text content
		if !m.isValidTextCSV(buf) {
			request.WriteJSON(w, http.StatusBadRequest, request.Fail(
				"INVALID_FILE_FORMAT",
				"File does not appear to be a valid CSV",
				nil,
			))
			return false
		}
	}

	// Validate CSV structure if enabled
	if m.config.ValidateCSV {
		if err := m.validateCSVStructure(buf); err != nil {
			request.WriteJSON(w, http.StatusBadRequest, request.Fail(
				"INVALID_CSV_FORMAT",
				err.Error(),
				nil,
			))
			return false
		}
	}

	return true
}

// isAllowedMime checks if MIME type is allowed
func isAllowedMime(allowedMimeTypes []string, mimeType string) bool {
	for _, allowed := range allowedMimeTypes {
		if strings.HasPrefix(mimeType, allowed) {
			return true
		}
	}
	return false
}

// hasValidMagicBytes checks for valid magic bytes
func (m *FileUploadMiddleware) hasValidMagicBytes(buf []byte) bool {
	// Check for UTF-8 BOM
	for _, magic := range csvMagicBytes {
		if len(buf) >= len(magic) {
			match := true
			for i, b := range magic {
				if buf[i] != b {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// isValidTextCSV checks if content looks like valid CSV text
func (m *FileUploadMiddleware) isValidTextCSV(buf []byte) bool {
	content := string(buf)

	// Check for CSV-like content
	hasComma := strings.Contains(content, ",")
	hasNewline := strings.Contains(content, "\n")

	if !hasComma || !hasNewline {
		return false
	}

	// Check for known CSV headers
	for _, signature := range textSignatures {
		if strings.Contains(strings.ToLower(content), signature) {
			return true
		}
	}

	return true
}

// validateCSVStructure validates CSV structure
func (m *FileUploadMiddleware) validateCSVStructure(buf []byte) error {
	reader := csv.NewReader(strings.NewReader(string(buf)))

	// Read header
	header, err := reader.Read()
	if err != nil {
		return ErrMalformedCSV
	}

	if len(header) == 0 {
		return errors.New("CSV file has no headers")
	}

	// Validate header contains expected columns
	hasRequiredColumn := false
	for _, col := range header {
		col = strings.ToLower(strings.TrimSpace(col))
		for _, signature := range textSignatures {
			if col == signature || strings.Contains(col, signature) {
				hasRequiredColumn = true
				break
			}
		}
		if hasRequiredColumn {
			break
		}
	}

	if !hasRequiredColumn {
		return errors.New("CSV file missing required columns")
	}

	return nil
}

// ContentTypeMiddleware sets security headers for Content-Type
func ContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Set default content type for JSON responses
		if r.Header.Get("Accept") == "" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}

		next.ServeHTTP(w, r)
	})
}
