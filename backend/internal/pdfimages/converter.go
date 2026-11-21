package pdfimages

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxUploadSize     = 20 << 20
	maxRequestSize    = maxUploadSize + 1<<20
	maxPages          = 10
	renderDPI         = 150
	conversionTimeout = 60 * time.Second
)

var (
	errInvalidPDF       = errors.New("invalid PDF")
	errConversionFailed = errors.New("PDF conversion failed")
	errToolUnavailable  = errors.New("PDF conversion tool unavailable")
)

type Converter interface {
	PageCount(context.Context, string) (int, error)
	Render(context.Context, string, string, int) error
}

type CommandConverter struct {
	pdfInfoPath string
	muToolPath  string
}

func NewCommandConverter() *CommandConverter {
	return &CommandConverter{pdfInfoPath: "pdfinfo", muToolPath: "mutool"}
}

func (c *CommandConverter) PageCount(ctx context.Context, pdfPath string) (int, error) {
	command := exec.CommandContext(ctx, c.pdfInfoPath, pdfPath)
	output, err := command.CombinedOutput()
	if err != nil {
		if isToolUnavailable(err) {
			return 0, fmt.Errorf("%w: %v", errToolUnavailable, err)
		}
		return 0, fmt.Errorf("%w: %s", errInvalidPDF, strings.TrimSpace(string(output)))
	}
	pageCount, err := parsePageCount(string(output))
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errInvalidPDF, err)
	}
	return pageCount, nil
}

func (c *CommandConverter) Render(ctx context.Context, pdfPath, outputPattern string, pageCount int) error {
	command := exec.CommandContext(
		ctx,
		c.muToolPath,
		"draw",
		"-r", strconv.Itoa(renderDPI),
		"-o", outputPattern,
		pdfPath,
		fmt.Sprintf("1-%d", pageCount),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		if isToolUnavailable(err) {
			return fmt.Errorf("%w: %v", errToolUnavailable, err)
		}
		return fmt.Errorf("%w: %s", errConversionFailed, strings.TrimSpace(string(output)))
	}
	return nil
}

func isToolUnavailable(err error) bool {
	var execErr *exec.Error
	return errors.As(err, &execErr) || errors.Is(err, exec.ErrNotFound)
}

func parsePageCount(output string) (int, error) {
	for _, line := range strings.Split(output, "\n") {
		name, value, found := strings.Cut(line, ":")
		if found && strings.TrimSpace(name) == "Pages" {
			count, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || count < 1 {
				return 0, fmt.Errorf("invalid page count")
			}
			return count, nil
		}
	}
	return 0, fmt.Errorf("page count not found")
}

type convertResponse struct {
	Filename  string   `json:"filename"`
	PageCount int      `json:"page_count"`
	Images    []string `json:"images"`
}

func NewHandler(converter Converter, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/convert" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleConvert(w, r, converter, logger)
	})
}

func handleConvert(w http.ResponseWriter, r *http.Request, converter Converter, logger *slog.Logger) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
	parseErr := r.ParseMultipartForm(maxUploadSize)
	if r.MultipartForm != nil {
		defer func() {
			if err := r.MultipartForm.RemoveAll(); err != nil {
				logger.Error("remove multipart temporary files", "error", err)
			}
		}()
	}
	if parseErr != nil {
		http.Error(w, "Invalid multipart request or file too large", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing PDF file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if header.Size > maxUploadSize {
		http.Error(w, "PDF file exceeds 20 MB", http.StatusBadRequest)
		return
	}
	filename := filepath.Base(header.Filename)
	if !strings.EqualFold(filepath.Ext(filename), ".pdf") {
		http.Error(w, "Only PDF files supported", http.StatusBadRequest)
		return
	}

	if err := convertFile(w, r, file, filename, converter, logger); err != nil {
		writeConversionError(w, err, logger)
	}
}

func convertFile(
	w http.ResponseWriter,
	r *http.Request,
	file io.Reader,
	filename string,
	converter Converter,
	logger *slog.Logger,
) error {
	tempDir, err := os.MkdirTemp("", "pdf-converter-*")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			logger.Error("remove conversion temporary directory", "error", err)
		}
	}()

	pdfPath := filepath.Join(tempDir, "input.pdf")
	if err := saveUpload(pdfPath, file); err != nil {
		return fmt.Errorf("save PDF upload: %w", err)
	}

	ctx, cancel := context.WithTimeout(r.Context(), conversionTimeout)
	defer cancel()
	pageCount, err := converter.PageCount(ctx, pdfPath)
	if err != nil {
		return err
	}
	if pageCount < 1 {
		return errInvalidPDF
	}
	if pageCount > maxPages {
		pageCount = maxPages
	}

	outputPattern := filepath.Join(tempDir, "page-%d.png")
	if err := converter.Render(ctx, pdfPath, outputPattern, pageCount); err != nil {
		return err
	}

	images := make([]string, 0, pageCount)
	for page := 1; page <= pageCount; page++ {
		imagePath := fmt.Sprintf(outputPattern, page)
		imageBytes, err := os.ReadFile(imagePath)
		if err != nil {
			return fmt.Errorf("read converted page %d: %w", page, err)
		}
		images = append(images, base64.StdEncoding.EncodeToString(imageBytes))
	}

	writeJSON(w, http.StatusOK, convertResponse{
		Filename:  filename,
		PageCount: len(images),
		Images:    images,
	}, logger)
	return nil
}

func saveUpload(path string, file io.Reader) error {
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, file); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func writeConversionError(w http.ResponseWriter, err error, logger *slog.Logger) {
	switch {
	case errors.Is(err, errInvalidPDF):
		http.Error(w, "Invalid PDF or PDF contains no pages", http.StatusBadRequest)
	case errors.Is(err, errConversionFailed):
		http.Error(w, "Failed to convert PDF to images", http.StatusBadRequest)
	case errors.Is(err, errToolUnavailable):
		logger.Error("PDF conversion tools unavailable", "error", err)
		http.Error(w, "PDF conversion tools unavailable", http.StatusInternalServerError)
	default:
		logger.Error("PDF conversion failed", "error", err)
		http.Error(w, "Failed to process PDF", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any, logger *slog.Logger) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.Error("write JSON response", "error", err)
	}
}
