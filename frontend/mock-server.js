import { createServer } from "node:http";

const HOST = "127.0.0.1";
const PORT = 8081;
const MAX_BODY_BYTES = 10 * 1024 * 1024;
let nextId = 1043;
const processingStatuses = new Set([
  "awaiting_po", "processing", "verified", "mismatch",
  "needs_review", "error", "not_applicable",
]);

const invoices = [
  {
    id: 1042, vendor_name: "Northwind Supplies", invoice_number: "INV-2026-041",
    invoice_date: "2026-11-18", po_number: "PO-8872", due_date: "2026-12-18",
    grand_total: 12540, subtotal: 10800, tax_amount: 1740, currency: "INR",
    is_invoice: true, processing_status: "awaiting_po", created_at: "2026-11-19T10:24:00Z",
    email: {
      gmail_id: "mock-gmail-1042", thread_id: "mock-thread-1042", message_id: "mock-message-1042",
      subject: "Invoice INV-2026-041", sender: "billing@northwind.example",
      recipient: "ap@yourcompany.example", email_date: "2026-11-19T10:20:00Z",
      body: "Hello Accounts Payable,\n\nPlease find invoice INV-2026-041 attached for processing.\n\nRegards,\nNorthwind Supplies",
    },
    extraction: { channel: "pdf", model: "mock-extractor", done: true },
    line_items: [{ description: "Office equipment", quantity: 2, unit: "unit", unit_price: 5400, line_total: 10800 }],
  },
  {
    id: 1041, vendor_name: "Contoso Logistics", invoice_number: "INV-9918",
    invoice_date: "2026-11-16", po_number: "PO-8849", due_date: "2026-12-16",
    grand_total: 8420.5, subtotal: 7136.02, tax_amount: 1284.48, currency: "INR",
    is_invoice: true, processing_status: "awaiting_po", created_at: "2026-11-17T08:15:00Z",
    email: {
      gmail_id: "mock-gmail-1041", thread_id: "mock-thread-1041", message_id: "mock-message-1041",
      subject: "November freight", sender: "accounts@contoso.example",
      recipient: "ap@yourcompany.example", email_date: "2026-11-17T08:12:00Z",
      body: "Attached is our November freight invoice INV-9918.",
    },
    extraction: { channel: "pdf", model: "mock-extractor", done: true },
    line_items: [{ description: "Freight services", quantity: 1, unit: "service", unit_price: 7136.02, line_total: 7136.02 }],
  },
  {
    id: 1040, vendor_name: "Fabrikam Office", invoice_number: "INV-4412",
    invoice_date: "2026-11-12", po_number: "", due_date: "2026-12-12",
    grand_total: 2190, subtotal: 1855.93, tax_amount: 334.07, currency: "INR",
    is_invoice: true, processing_status: "not_applicable", created_at: "2026-11-13T14:02:00Z",
    email: {
      gmail_id: "mock-gmail-1040", thread_id: "mock-thread-1040", message_id: "mock-message-1040",
      subject: "Invoice INV-4412", sender: "billing@fabrikam.example",
      recipient: "ap@yourcompany.example", email_date: "2026-11-13T13:58:00Z",
      body: "Please see the attached invoice for stationery supplies.",
    },
    extraction: { channel: "img", model: "mock-extractor", done: true },
    line_items: [{ description: "Stationery", quantity: 10, unit: "pack", unit_price: 185.593, line_total: 1855.93 }],
  },
  {
    id: 1039, vendor_name: "Adatum Services", invoice_number: "",
    invoice_date: "2026-11-10", po_number: "PO-8791", due_date: "",
    grand_total: 0, subtotal: 0, tax_amount: 0, currency: "INR",
    is_invoice: false, processing_status: "not_applicable", created_at: "2026-11-11T09:45:00Z",
    email: {
      gmail_id: "mock-gmail-1039", thread_id: "mock-thread-1039", message_id: "mock-message-1039",
      subject: "Service document", sender: "hello@adatum.example",
      recipient: "ap@yourcompany.example", email_date: "2026-11-11T09:42:00Z",
      body: "Please review the attached service document.",
    },
    extraction: { channel: "pdf", model: "mock-extractor", done: true },
    line_items: [],
  },
];

function sendJson(response, status, body) {
  response.writeHead(status, {
    "Content-Type": "application/json; charset=utf-8",
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type, Idempotency-Key",
  });
  response.end(JSON.stringify(body));
}

function readBody(request) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    let tooLarge = false;
    request.on("data", (chunk) => {
      size += chunk.length;
      if (size > MAX_BODY_BYTES) {
        tooLarge = true;
        return;
      }
      chunks.push(chunk);
    });
    request.on("end", () => {
      if (tooLarge) reject(Object.assign(new Error("Request body exceeds 10 MB."), { status: 413 }));
      else resolve(Buffer.concat(chunks));
    });
    request.on("error", reject);
  });
}

function summary(invoice) {
  const { id, vendor_name, invoice_number, invoice_date, po_number, grand_total, currency, is_invoice, processing_status, created_at } = invoice;
  return { id, vendor_name, invoice_number, invoice_date, po_number, grand_total, currency, is_invoice, processing_status, created_at };
}

function initialProcessingStatus(invoice) {
  if (invoice.is_invoice === false || !String(invoice.po_number ?? "").trim()) {
    return "not_applicable";
  }
  return "awaiting_po";
}

function processingStatusLabel(status) {
  const labels = {
    awaiting_po: "Awaiting PO",
    processing: "Verifying PO",
    verified: "Verified",
    mismatch: "PO mismatch",
    needs_review: "Needs review",
    error: "Verification error",
    not_applicable: "Not applicable",
  };
  return Object.hasOwn(labels, status) ? labels[status] : "Unknown";
}

function createInvoice(payload) {
  const gmailId = payload?.email?.gmail_id;
  if (!gmailId || !payload?.invoice || !payload?.extraction || !Array.isArray(payload?.line_items)) {
    return { status: 400, body: { err_code: "INVALID_INVOICE", err_msg: "email.gmail_id, extraction, invoice, and line_items are required." } };
  }

  const existing = invoices.find((invoice) => invoice.email.gmail_id === gmailId);
  if (existing) {
    return { status: 200, body: { success: true, invoice_id: existing.id, already_exists: true, message: "Invoice already exists." } };
  }

  const invoice = {
    ...payload.invoice,
    id: nextId++,
    processing_status: initialProcessingStatus(payload.invoice),
    created_at: new Date().toISOString(),
    email: payload.email,
    extraction: payload.extraction,
    line_items: Array.isArray(payload.line_items) ? payload.line_items : [],
  };
  invoices.unshift(invoice);
  return { status: 201, body: { success: true, invoice_id: invoice.id, already_exists: false, message: "Invoice created." } };
}

function filterInvoices(searchParams) {
  let result = [...invoices];
  const filters = ["vendor_name", "po_number", "invoice_number"];
  for (const key of filters) {
    const value = searchParams.get(key)?.trim().toLowerCase();
    if (value) result = result.filter((invoice) => String(invoice[key] || "").toLowerCase().includes(value));
  }
  const isInvoice = searchParams.get("is_invoice");
  if (isInvoice === "true" || isInvoice === "false") {
    result = result.filter((invoice) => invoice.is_invoice === (isInvoice === "true"));
  }
  const processingStatus = searchParams.get("processing_status");
  if (processingStatus) {
    if (!processingStatuses.has(processingStatus)) {
      return { error: "processing_status must be a supported status value." };
    }
    result = result.filter((invoice) => invoice.processing_status === processingStatus);
  }
  const fromDate = searchParams.get("from_date");
  const toDate = searchParams.get("to_date");
  if (fromDate) result = result.filter((invoice) => invoice.invoice_date >= fromDate);
  if (toDate) result = result.filter((invoice) => invoice.invoice_date <= toDate);

  const page = Number(searchParams.get("page") || 1);
  const limit = Number(searchParams.get("limit") || 20);
  if (!Number.isInteger(page) || page < 1 || !Number.isInteger(limit) || limit < 1 || limit > 100) {
    return { error: "page must be >= 1 and limit must be between 1 and 100." };
  }
  const total = result.length;
  return { total, page, limit, invoices: result.slice((page - 1) * limit, page * limit).map(summary) };
}

function csvCell(value) {
  const text = String(value ?? "");
  return `"${text.replaceAll('"', '""')}"`;
}

function amountInMinorUnits(value) {
  const normalized = String(value ?? "").trim().replaceAll(",", "");
  if (!/^-?\d+(?:\.\d{1,2})?$/.test(normalized)) return null;
  const [whole, fraction = ""] = normalized.replace("-", "").split(".");
  const amount = BigInt(whole) * 100n + BigInt(fraction.padEnd(2, "0"));
  return normalized.startsWith("-") ? -amount : amount;
}

function generatePOCsv(workbook) {
  if (!Array.isArray(workbook.sheets) || workbook.sheets.length === 0) {
    throw new Error("The request must include at least one parsed sheet.");
  }
  const columns = new Map();
  for (const sheet of workbook.sheets) {
    if (!Array.isArray(sheet.rows) || sheet.rows.length === 0) continue;
    const keys = new Set(sheet.columns.map((column) => column.key));
    for (const required of ["po_number", "vendor", "po_amount", "currency"]) {
      if (!keys.has(required)) throw new Error(`Sheet "${sheet.name}" is missing the required ${required} column.`);
    }
    for (const column of sheet.columns) {
      if (column.key !== "automation_status" && !columns.has(column.key)) {
        columns.set(column.key, column.label);
      }
    }
  }

  const poCounts = new Map();
  for (const sheet of workbook.sheets) {
    for (const row of sheet.rows || []) {
      const poNumber = String(row.values?.po_number || "").trim().toUpperCase();
      if (poNumber) poCounts.set(poNumber, (poCounts.get(poNumber) || 0) + 1);
    }
  }

  const headers = ["source_sheet", ...columns.values(), "automation_status"];
  const outputRows = [];
  for (const sheet of workbook.sheets) {
    for (const row of sheet.rows || []) {
      const values = row.values || {};
      const poNumber = String(values.po_number || "").trim();
      const poAmount = amountInMinorUnits(values.po_amount);
      const poInvoices = poNumber ? invoices.filter((invoice) =>
        invoice.is_invoice === true &&
        String(invoice.po_number || "").trim().toLowerCase() === poNumber.toLowerCase()
      ) : [];
      const candidates = poInvoices.filter((invoice) =>
        String(invoice.vendor_name || "").trim().toLowerCase() === String(values.vendor || "").trim().toLowerCase() &&
        String(invoice.currency || "").trim().toLowerCase() === String(values.currency || "").trim().toLowerCase()
      );
      let status = "needs_review";
      if (poNumber && poCounts.get(poNumber.toUpperCase()) === 1 && poAmount !== null &&
          String(values.vendor || "").trim() && String(values.currency || "").trim() && candidates.length > 0) {
        const totals = candidates.map((invoice) => amountInMinorUnits(invoice.grand_total));
        if (totals.every((total) => total !== null)) {
          const sum = totals.reduce((total, amount) => total + amount, 0n);
          status = sum === poAmount ? "verified" : "mismatch";
          candidates.forEach((invoice) => { invoice.processing_status = status; });
        }
      }
      if (status === "needs_review") {
        poInvoices.forEach((invoice) => { invoice.processing_status = status; });
      }
      outputRows.push([
        sheet.name,
        ...[...columns.keys()].map((key) => values[key] || ""),
        status,
      ].map(csvCell).join(","));
    }
  }
  return [headers.map(csvCell).join(","), ...outputRows].join("\r\n");
}

const server = createServer(async (request, response) => {
  if (request.method === "OPTIONS") {
    response.writeHead(204, {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type, Idempotency-Key",
      "Access-Control-Expose-Headers": "Content-Disposition",
    });
    response.end();
    return;
  }

  const url = new URL(request.url, `http://${HOST}:${PORT}`);
  const path = url.pathname;
  try {
    if (request.method === "GET" && path === "/api/v1/invoice.list") {
      const result = filterInvoices(url.searchParams);
      if (result.error) return sendJson(response, 400, { err_code: "INVALID_QUERY", err_msg: result.error });
      return sendJson(response, 200, result);
    }

    if (request.method === "GET" && path === "/api/v1/invoice.get") {
      const id = Number(url.searchParams.get("id"));
      if (!Number.isInteger(id)) return sendJson(response, 400, { err_code: "INVALID_ID", err_msg: "A numeric id query parameter is required." });
      const invoice = invoices.find((record) => record.id === id);
      if (!invoice) return sendJson(response, 404, { err_code: "NOT_FOUND", err_msg: "Invoice not found." });
      return sendJson(response, 200, {
        invoice: summary(invoice),
        extraction: invoice.extraction,
        email: invoice.email,
        line_items: invoice.line_items,
      });
    }

    if (request.method === "POST" && path === "/api/v1/invoice.create") {
      const body = await readBody(request);
      let payload;
      try {
        payload = JSON.parse(body.toString("utf8"));
      } catch {
        return sendJson(response, 400, { err_code: "INVALID_JSON", err_msg: "Request body must be valid JSON." });
      }
      const result = createInvoice(payload);
      return sendJson(response, result.status, result.body);
    }

    if (request.method === "POST" && path === "/api/v1/invoice.bulk-create") {
      const body = await readBody(request);
      let payload;
      try {
        payload = JSON.parse(body.toString("utf8"));
      } catch {
        return sendJson(response, 400, { err_code: "INVALID_JSON", err_msg: "Request body must be valid JSON." });
      }
      if (!Array.isArray(payload?.invoices) || payload.invoices.length < 1 || payload.invoices.length > 100) {
        return sendJson(response, 400, { err_code: "INVALID_BATCH", err_msg: "invoices must contain between 1 and 100 records." });
      }
      const results = payload.invoices.map((item) => {
        const result = createInvoice(item);
        const gmailId = item?.email?.gmail_id || "";
        if (result.status >= 400) {
          return { gmail_id: gmailId, status: "failed", err_code: result.body.err_code, err_msg: result.body.err_msg };
        }
        return {
          gmail_id: gmailId,
          status: result.body.already_exists ? "already_exists" : "created",
          invoice_id: result.body.invoice_id,
        };
      });
      return sendJson(response, 200, {
        total: results.length,
        created: results.filter((item) => item.status === "created").length,
        already_exists: results.filter((item) => item.status === "already_exists").length,
        failed: results.filter((item) => item.status === "failed").length,
        results,
      });
    }

    if (request.method === "POST" && path === "/api/v1/report.gen") {
      const body = await readBody(request);
      let workbook;
      try {
        workbook = JSON.parse(body.toString("utf8"));
      } catch {
        return sendJson(response, 400, { err_code: "INVALID_REQUEST", err_msg: "Send parsed workbook data as JSON." });
      }
      let csv;
      try {
        csv = generatePOCsv(workbook);
      } catch (error) {
        return sendJson(response, 400, { err_code: "INVALID_WORKBOOK", err_msg: error.message });
      }

      response.writeHead(200, {
        "Content-Type": "text/csv; charset=utf-8",
        "Content-Disposition": `attachment; filename="mock-purchase-order-reconciliation.csv"`,
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Expose-Headers": "Content-Disposition",
      });
      return response.end(csv);
    }

    return sendJson(response, 404, { err_code: "NOT_FOUND", err_msg: "Mock API route not found." });
  } catch (error) {
    if (!response.headersSent) {
      sendJson(response, error.status || 500, { err_code: "MOCK_API_ERROR", err_msg: error.message || "Mock API request failed." });
    } else {
      response.destroy(error);
    }
  }
});

server.listen(PORT, HOST, () => {
  console.log(`Mock Invoice API listening at http://${HOST}:${PORT}/api/v1`);
});
