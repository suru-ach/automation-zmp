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

function multipartFields(body, contentType) {
  const boundary = contentType.match(/boundary=(?:"([^"]+)"|([^;]+))/i);
  if (!boundary) return null;
  const separator = `--${(boundary[1] || boundary[2]).trim()}`;
  const parts = body.toString("latin1").split(separator);
  const fields = {};
  let hasFile = false;

  for (const part of parts) {
    const headerEnd = part.indexOf("\r\n\r\n");
    if (headerEnd < 0) continue;
    const headers = part.slice(0, headerEnd);
    const name = headers.match(/name="([^"]+)"/i)?.[1];
    if (!name) continue;
    if (name === "file") {
      hasFile = Boolean(headers.match(/filename="[^"]+"/i));
    } else {
      fields[name] = part.slice(headerEnd + 4).replace(/\r\n--?\s*$/, "").replace(/\r\n$/, "");
    }
  }
  return { fields, hasFile };
}

function csvCell(value) {
  const text = String(value ?? "");
  return `"${text.replaceAll('"', '""')}"`;
}

function generateCsv(fields) {
  const rows = invoices.filter((invoice) => {
    if (fields.from_date && invoice.invoice_date < fields.from_date) return false;
    if (fields.to_date && invoice.invoice_date > fields.to_date) return false;
    return true;
  });
  const lines = [
    ["Invoice ID", "Vendor", "Invoice Number", "Invoice Date", "PO Number", "Currency", "Total", "Status"].map(csvCell).join(","),
    ...rows.map((invoice) => [
      invoice.id, invoice.vendor_name, invoice.invoice_number, invoice.invoice_date,
      invoice.po_number, invoice.currency, invoice.grand_total, processingStatusLabel(invoice.processing_status),
    ].map(csvCell).join(",")),
  ];
  return lines.join("\r\n");
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
      const contentType = request.headers["content-type"] || "";
      const multipart = multipartFields(body, contentType);
      if (!multipart?.hasFile) {
        return sendJson(response, 400, { err_code: "FILE_REQUIRED", err_msg: "Upload a purchase order file in the file field." });
      }
      const { fields } = multipart;
      const fromDate = fields.from_date || "";
      const toDate = fields.to_date || "";
      if ((fromDate && !/^\d{4}-\d{2}-\d{2}$/.test(fromDate)) || (toDate && !/^\d{4}-\d{2}-\d{2}$/.test(toDate))) {
        return sendJson(response, 400, { err_code: "INVALID_DATE", err_msg: "Report dates must use YYYY-MM-DD format." });
      }

      response.writeHead(200, {
        "Content-Type": "text/csv; charset=utf-8",
        "Content-Disposition": `attachment; filename="mock-invoice-report-${new Date().toISOString().slice(0, 10)}.csv"`,
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Expose-Headers": "Content-Disposition",
      });
      return response.end(generateCsv(fields));
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
