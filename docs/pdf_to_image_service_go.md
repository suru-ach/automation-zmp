# PDF-to-Image Conversion Service (Go)

## 1. Purpose

A small HTTP microservice that accepts PDF uploads and converts up to the first 10 pages into PNG images at 150 DPI. The PNG images are Base64-encoded and returned in JSON, so an automation workflow (such as n8n) can pass them to a vision model for invoice extraction.

## 2. API Design

### `GET /health`

Health check.

**Response — `200 OK`**

```json
{
  "status": "ok"
}
```

### `POST /convert`

Accepts a PDF file as `multipart/form-data`, using the field name `file`.

**Rules**
- Only filenames ending in `.pdf` are accepted (case-insensitive).
- Maximum upload size: 20 MB.
- Converts at most the first 10 pages.
- Output: PNG images, 150 DPI, Base64-encoded.
- Temporary files are removed after the request.

**Response — `200 OK`**

```json
{
  "filename": "invoice.pdf",
  "page_count": 2,
  "images": [
    "<base64 PNG page 1>",
    "<base64 PNG page 2>"
  ]
}
```

**Errors**
- `400 Bad Request`: missing/invalid multipart upload, unsupported file extension, invalid PDF, no pages, or conversion failure.
- `405 Method Not Allowed`: method other than POST for `/convert`.
- `500 Internal Server Error`: temporary directory or image-read/write failure.

## 3. Architecture

```text
n8n
  |
  | POST /convert (multipart PDF)
  v
Go HTTP Service
  |
  +-- Validate method, upload, filename, and size
  |
  +-- Save PDF to a temporary directory
  |
  +-- pdfinfo: inspect page count
  |
  +-- mutool: render pages 1..min(total, 10) at 150 DPI
  |
  +-- Read PNG files and Base64-encode
  |
  +-- Return JSON
  |
  +-- Delete temporary directory
```

### Dependencies

- Go standard library for HTTP, multipart uploads, JSON, Base64, temporary files, and process execution.
- `pdfinfo` (Poppler utilities) to read PDF page count.
- `mutool` (MuPDF tools) to render PDF pages to PNG.

## 4. Project Structure

```text
pdf-converter/
├── main.go
├── go.mod
└── Dockerfile
```

## 5. Implementation

### `go.mod`

```go
module pdf-converter

go 1.23
```

### `main.go`

```go
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxUploadSize = 20 << 20 // 20 MB
	maxPages      = 10
	dpi           = 150
)

type ConvertResponse struct {
	Filename  string   `json:"filename"`
	PageCount int      `json:"page_count"`
	Images    []string `json:"images"`
}

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/convert", convertHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("PDF converter listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func convertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "Invalid multipart request or file too large", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing PDF file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := header.Filename
	if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		http.Error(w, "Only PDF files supported", http.StatusBadRequest)
		return
	}

	tempDir, err := os.MkdirTemp("", "pdf-converter-*")
	if err != nil {
		http.Error(w, "Failed to create temporary directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir)

	pdfPath := filepath.Join(tempDir, "input.pdf")
	out, err := os.Create(pdfPath)
	if err != nil {
		http.Error(w, "Failed to save PDF", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, "Failed to write PDF", http.StatusInternalServerError)
		return
	}
	if err := out.Close(); err != nil {
		http.Error(w, "Failed to close PDF file", http.StatusInternalServerError)
		return
	}

	infoCmd := exec.Command("pdfinfo", pdfPath)
	infoOutput, err := infoCmd.Output()
	if err != nil {
		http.Error(w, "Invalid PDF or failed to read PDF metadata", http.StatusBadRequest)
		return
	}

	totalPages, err := getPageCount(string(infoOutput))
	if err != nil {
		http.Error(w, "Failed to determine PDF page count", http.StatusBadRequest)
		return
	}
	if totalPages <= 0 {
		http.Error(w, "PDF contains no pages", http.StatusBadRequest)
		return
	}

	pagesToConvert := totalPages
	if pagesToConvert > maxPages {
		pagesToConvert = maxPages
	}

	outputPattern := filepath.Join(tempDir, "page-%d.png")
	renderCmd := exec.Command(
		"mutool",
		"draw",
		"-r", strconv.Itoa(dpi),
		"-o", outputPattern,
		pdfPath,
		fmt.Sprintf("1-%d", pagesToConvert),
	)
	if err := renderCmd.Run(); err != nil {
		http.Error(w, "Failed to convert PDF to images", http.StatusBadRequest)
		return
	}

	images := make([]string, 0, pagesToConvert)
	for page := 1; page <= pagesToConvert; page++ {
		imagePath := filepath.Join(tempDir, fmt.Sprintf("page-%d.png", page))
		imageBytes, err := os.ReadFile(imagePath)
		if err != nil {
			http.Error(w, "Failed to read converted image", http.StatusInternalServerError)
			return
		}

		encoded := base64.StdEncoding.EncodeToString(imageBytes)
		images = append(images, encoded)
	}

	response := ConvertResponse{
		Filename:  filename,
		PageCount: len(images),
		Images:    images,
	}
	writeJSON(w, http.StatusOK, response)
}

func getPageCount(pdfInfo string) (int, error) {
	for _, line := range strings.Split(pdfInfo, "\n") {
		if strings.HasPrefix(line, "Pages:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "Pages:"))
			return strconv.Atoi(value)
		}
	}
	return 0, fmt.Errorf("page count not found")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
	}
}
```

### `Dockerfile`

```dockerfile
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN go build -o pdf-converter main.go

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        mupdf-tools \
        poppler-utils \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/pdf-converter .

EXPOSE 8080

CMD ["./pdf-converter"]
```

## 6. Run Locally

Build the Docker image:

```bash
docker build -t pdf-converter .
```

Run the container:

```bash
docker run --rm -p 8080:8080 pdf-converter
```

Check health:

```bash
curl http://localhost:8080/health
```

Convert a PDF:

```bash
curl -X POST http://localhost:8080/convert \\
  -F "file=@invoice.pdf"
```

## 7. n8n Integration

In an n8n HTTP Request node:

- **Method:** `POST`
- **URL:** `https://<your-service-host>/convert`
- **Body Content Type:** `Multipart Form Data`
- **Form field name:** `file`
- **Value:** the binary PDF attachment from the previous node

The API responds with JSON containing an array of Base64 PNG images. Pass those images to the vision-model node, adapting the image input format expected by the model/provider.

## 8. Notes / Production Hardening

- Configure an upload size limit appropriate to your use case.
- Consider adding request timeouts and concurrency limits.
- Validate the actual PDF content, not just the filename extension.
- Consider authentication (API key or service-to-service auth) before exposing this service publicly.
- Rendering PDFs can consume significant memory and CPU; consider stricter page-count and image-size limits.
- Keep the service updated with security fixes for MuPDF and Poppler.
- Base64 adds roughly 33% size overhead; large responses may be better handled as image files or object-storage references in a later iteration.
