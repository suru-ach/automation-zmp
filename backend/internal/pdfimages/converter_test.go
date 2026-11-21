package pdfimages

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeConverter struct {
	pageCount     int
	renderedPages int
}

func (f *fakeConverter) PageCount(context.Context, string) (int, error) {
	return f.pageCount, nil
}

func (f *fakeConverter) Render(_ context.Context, _, outputPattern string, pageCount int) error {
	f.renderedPages = pageCount
	for page := 1; page <= pageCount; page++ {
		if err := os.WriteFile(fmt.Sprintf(outputPattern, page), []byte(fmt.Sprintf("png-%d", page)), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func TestConvertReturnsBase64ImagesAndCapsPages(t *testing.T) {
	converter := &fakeConverter{pageCount: 12}
	handler := NewHandler(converter, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := pdfUploadRequest(t, "invoice.PDF", []byte("pdf"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if converter.renderedPages != maxPages {
		t.Fatalf("rendered pages = %d, want %d", converter.renderedPages, maxPages)
	}
	var result convertResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Filename != "invoice.PDF" || result.PageCount != maxPages || len(result.Images) != maxPages {
		t.Fatalf("unexpected conversion response: %+v", result)
	}
	decoded, err := base64.StdEncoding.DecodeString(result.Images[0])
	if err != nil || string(decoded) != "png-1" {
		t.Fatalf("unexpected Base64 image: decoded=%q err=%v", decoded, err)
	}
}

func TestConvertRejectsUnsupportedFileAndNonPostMethod(t *testing.T) {
	handler := NewHandler(&fakeConverter{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	unsupported := httptest.NewRecorder()
	handler.ServeHTTP(unsupported, pdfUploadRequest(t, "invoice.txt", []byte("data")))
	if unsupported.Code != http.StatusBadRequest {
		t.Fatalf("unsupported extension status = %d, want %d", unsupported.Code, http.StatusBadRequest)
	}

	methodRequest := httptest.NewRequest(http.MethodGet, "/api/v1/convert", nil)
	methodResponse := httptest.NewRecorder()
	handler.ServeHTTP(methodResponse, methodRequest)
	if methodResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want %d", methodResponse.Code, http.StatusMethodNotAllowed)
	}
}

func TestConvertMapsInvalidPDFToBadRequest(t *testing.T) {
	handler := NewHandler(&fakeConverter{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, pdfUploadRequest(t, "invoice.pdf", []byte("not a PDF")))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestParsePageCount(t *testing.T) {
	count, err := parsePageCount("Title: invoice.pdf\nPages: 3\n")
	if err != nil || count != 3 {
		t.Fatalf("parsePageCount() = (%d, %v), want (3, nil)", count, err)
	}
	if _, err := parsePageCount("Title: invoice.pdf\n"); err == nil {
		t.Fatal("expected missing page count to fail")
	}
}

func pdfUploadRequest(t *testing.T, filename string, data []byte) *http.Request {
	t.Helper()
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/convert", strings.NewReader(body.String()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
