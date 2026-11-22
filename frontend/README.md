# Invoice Desk

A small vanilla JavaScript dashboard for the Invoice Processing API.

## Run with the mock API

```bash
cd frontend
npm install
npm run mock-api
```

In another terminal, start the dashboard:

```bash
cd frontend
npm run dev
```

The mock server listens on `http://localhost:8081/api/v1` and serves in-memory
invoice data. Its report endpoint returns a CSV mock download; only the real
Go API produces an annotated Excel workbook and persists invoice status
updates. Mock data resets whenever the server restarts.

The frontend defaults to the mock API. To connect to the real API instead,
create `.env` with:

```dotenv
VITE_API_BASE_URL=http://localhost:8080/api/v1
```

## Pages

- **Invoices** calls `GET /invoice.list` with vendor, invoice number, purchase
  order, date, page, and limit filters. Select an invoice number to load its
  email, extraction metadata, invoice fields, and line items from
  `GET /invoice.get`. If the API is unavailable, it visibly displays demo data.
- **Reports** parses CSV/XLSX files in the browser, previews normalized rows,
  then sends those rows as JSON to `POST /report.gen`. The real API matches PO
  number, vendor, and currency, requires an exact total match, updates matched
  invoice processing statuses (including `needs_review` for unresolved rows),
  and returns an XLSX with `automation_status`.
  The mock API returns a CSV sample.
