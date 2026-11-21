

# Backend

The Go API currently implements the invoice-list endpoint used by the
dashboard and PDF conversion endpoint used by the extraction workflow:

```text
GET http://localhost:8081/api/v1/invoice.list
POST http://localhost:8081/api/v1/convert
```

Invoice listing supports the OpenAPI query filters and reads from the
PostgreSQL tables in `../init.sql`. PDF conversion runs in the same API
process and requires `pdfinfo` and `mutool` on the host.

## Run locally

To start PostgreSQL and the Go API together with Docker Compose, set a
URL-safe development password and run from the repository root:

```bash
POSTGRES_PASSWORD=choose-a-local-password docker compose up --build
```

Compose runs only PostgreSQL and the backend. It runs `../init.sql` against a
new database volume and waits for PostgreSQL to become healthy before starting
the API. Stop the mock API first because it also uses port `8081`.

The database initialization script only runs when the `postgres_data` volume
is empty. To re-seed after changing the schema, back up any data you need,
then remove that volume explicitly.

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
psql "$DATABASE_URL" -f ../init.sql
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
