package reports

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

const maxRequestBytes = 10 << 20
const maxWorkbookSheets = 20
const maxWorkbookRows = 10000
const maxWorkbookColumns = 200
const maxExcelRow = 1048576

type Handler struct {
	generator Generator
	logger    *slog.Logger
}

func NewHandler(generator Generator, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{generator: generator, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Use POST to generate a report.")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Send parsed workbook data as application/json.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request WorkbookRequest
	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "The parsed workbook exceeds the 10 MB request limit.")
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "The request body must be valid workbook JSON.")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "The request body must contain one JSON object.")
		return
	}
	if err := validateRequest(&request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WORKBOOK", err.Error())
		return
	}

	workbook, err := h.generator.Generate(r.Context(), &request)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "generate purchase order report", "error", err)
		writeError(w, http.StatusInternalServerError, "REPORT_FAILED", "The report could not be generated.")
		return
	}
	if len(workbook) == 0 {
		h.logger.ErrorContext(r.Context(), "generate purchase order report", "error", "generator returned an empty workbook")
		writeError(w, http.StatusInternalServerError, "REPORT_FAILED", "The report could not be generated.")
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, reportFileName(request.FileName)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(workbook); err != nil {
		h.logger.ErrorContext(r.Context(), "write purchase order report response", "error", err)
	}
}

func validateRequest(request *WorkbookRequest) error {
	if strings.TrimSpace(request.FileName) == "" || len(request.FileName) > 255 {
		return errors.New("The request must include a file name of at most 255 characters.")
	}
	if len(request.Sheets) == 0 || len(request.Sheets) > maxWorkbookSheets {
		return fmt.Errorf("Include between 1 and %d sheets.", maxWorkbookSheets)
	}
	totalRows := 0
	for sheetIndex := range request.Sheets {
		sheet := &request.Sheets[sheetIndex]
		if strings.TrimSpace(sheet.Name) == "" {
			return fmt.Errorf("Sheet %d must have a name.", sheetIndex+1)
		}
		if sheet.HeaderRow == 0 {
			sheet.HeaderRow = 1
		}
		if sheet.HeaderRow < 1 || sheet.HeaderRow > maxExcelRow {
			return fmt.Errorf("Sheet %q has an invalid header row.", sheet.Name)
		}
		if len(sheet.Columns) > maxWorkbookColumns {
			return fmt.Errorf("Sheet %q exceeds the %d-column limit.", sheet.Name, maxWorkbookColumns)
		}
		if len(sheet.Rows) == 0 {
			continue
		}
		if len(sheet.Columns) == 0 {
			return fmt.Errorf("Sheet %q has rows but no columns.", sheet.Name)
		}

		columnKeys := make(map[string]bool, len(sheet.Columns))
		for _, column := range sheet.Columns {
			if strings.TrimSpace(column.Key) == "" || strings.TrimSpace(column.Label) == "" {
				return fmt.Errorf("Sheet %q has a column with a missing key or label.", sheet.Name)
			}
			if columnKeys[column.Key] {
				return fmt.Errorf("Sheet %q contains duplicate column keys.", sheet.Name)
			}
			columnKeys[column.Key] = true
		}
		for _, requiredColumn := range []string{"po_number", "vendor", "po_amount", "currency"} {
			if !columnKeys[requiredColumn] {
				return fmt.Errorf("Sheet %q is missing the required %q column.", sheet.Name, requiredColumn)
			}
		}

		sourceRows := make(map[int]bool, len(sheet.Rows))
		for _, row := range sheet.Rows {
			if row.SourceRow <= sheet.HeaderRow || row.SourceRow > maxExcelRow {
				return fmt.Errorf("Sheet %q contains an invalid source row.", sheet.Name)
			}
			if sourceRows[row.SourceRow] {
				return fmt.Errorf("Sheet %q contains duplicate source rows.", sheet.Name)
			}
			sourceRows[row.SourceRow] = true
			totalRows++
		}
	}
	if totalRows == 0 {
		return errors.New("The workbook contains no data rows.")
	}
	if totalRows > maxWorkbookRows {
		return fmt.Errorf("The workbook exceeds the %d-row limit.", maxWorkbookRows)
	}
	return nil
}

var safeFileName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func reportFileName(input string) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(input)), filepath.Ext(input))
	base = strings.Trim(safeFileName.ReplaceAllString(base, "-"), ".-_")
	if len(base) > 80 {
		base = base[:80]
	}
	if base == "" {
		base = "purchase-orders"
	}
	return base + "-reconciliation.xlsx"
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"err_code": code,
		"err_msg":  message,
	})
}
