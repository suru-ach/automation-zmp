# PDF-to-image processing

PDF conversion is implemented as a handler in the existing Go backend. It
does not start a separate server or use a separate port.

## Endpoint

```text
POST http://localhost:8081/api/v1/convert
```

Send a PDF in a `multipart/form-data` field named `file`.

```bash
curl -X POST http://localhost:8081/api/v1/convert \
  -F "file=@invoice.pdf"
```

The endpoint accepts `.pdf` filenames case-insensitively, limits uploads to
20 MB, and converts up to the first 10 pages to PNG at 150 DPI. The response
contains the uploaded filename, number of converted pages, and a Base64 PNG
string for each page:

```json
{
  "filename": "invoice.pdf",
  "page_count": 2,
  "images": ["<base64 PNG page 1>", "<base64 PNG page 2>"]
}
```

Invalid uploads, invalid PDFs, and conversion failures return `400`; missing
PDF tools or filesystem failures return `500`.

## Implementation and runtime requirements

- The route is registered in `backend/internal/api/handler.go`.
- Request validation and conversion are in `backend/internal/pdfimages/converter.go`.
- The existing API server runs both invoice endpoints and PDF conversion on
  port `8081`.
- Install Poppler (`pdfinfo`) and MuPDF (`mutool`) on the API host. On
  Debian/Ubuntu, install `poppler-utils` and `mupdf-tools`.
- Temporary upload and PNG files are removed after the request.

Run the backend tests from the `backend` directory with:

```bash
go test ./...
```
