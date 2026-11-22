

# Backend

The Go API currently implements the invoice-list endpoint used by the
dashboard and PDF conversion endpoint used by the extraction workflow:

```text
GET http://localhost:8081/api/v1/invoice.list
POST http://localhost:8081/api/v1/convert
```

Invoice listing supports the OpenAPI query filters and reads from the
PostgreSQL tables created by `../migrations/000_init.sql` and
`../migrations/001_add_invoice_processing_status.sql`. PDF conversion runs in the same API
process and requires `pdfinfo` and `mutool` on the host.

## Run locally

To start PostgreSQL and the Go API together with Docker Compose, set a
URL-safe development password and run from the repository root:

```bash
POSTGRES_PASSWORD=choose-a-local-password docker compose up --build
```

Compose runs only PostgreSQL and the backend. On a new database volume,
PostgreSQL runs the SQL files in `../migrations/` in filename order, then waits
for PostgreSQL to become healthy before starting the API. Stop the mock API
first because it also uses port `8081`.

The SQL scripts only run when the `postgres_data` volume is empty. For an
existing database, apply the new status migration manually from the repository
root:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 \
  -f migrations/001_add_invoice_processing_status.sql
```

The new column defaults existing rows to `awaiting_po`; update existing
processing statuses manually if needed.

To insert repeatable demonstration invoices after the stack is running:

```bash
POSTGRES_PASSWORD=localdev123 docker compose exec -T db \
  psql -v ON_ERROR_STOP=1 -U invoice_tracker -d invoice_tracker \
  < tmp/seed_dummy_data.sql
```

The seed script uses synthetic `.example` email addresses and skips records
whose demo Gmail IDs already exist.

Alternatively, run the Go API directly against an existing PostgreSQL
database:

```bash
cd backend
export DATABASE_URL='postgres://user:password@localhost:5432/invoices?sslmode=disable'
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f ../migrations/000_init.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f ../migrations/001_add_invoice_processing_status.sql
export ALLOWED_ORIGIN='http://localhost:5173'
go run .
```

The server defaults to port `8081`; set `PORT` to override it. The dashboard
also uses port `8081` for its mock API, so stop the mock server before running
the Go API on the same port.

Run the unit tests with:

```bash
go test ./...
```

For external testing, expose port `8081` through a tunnel such as ngrok and
set the frontend's `VITE_API_BASE_URL` to the tunnel's HTTPS URL followed by
`/api/v1`.
