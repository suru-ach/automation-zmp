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

The mock server listens on `http://localhost:8081/api/v1`, serves in-memory
invoice data, and supports all routes in the OpenAPI contract. Its report
endpoint returns a sample CSV download; it does not produce a real Excel
reconciliation workbook. Mock data resets whenever the server restarts.

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
- **Reports** calls `POST /report.gen` as `multipart/form-data` with `file`,
  `from_date`, and `to_date`, then downloads the report (CSV from the mock API,
  Excel from the real API).
