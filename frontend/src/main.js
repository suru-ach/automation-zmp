import "./styles.css";
import Papa from "papaparse";
import readExcelFile from "read-excel-file/browser";
import { generateReport, getInvoice, listInvoices } from "./api.js";

const app = document.querySelector("#app");

const processingStatusLabels = {
  awaiting_po: "Awaiting PO",
  processing: "Verifying PO",
  verified: "Verified",
  mismatch: "PO mismatch",
  needs_review: "Needs review",
  error: "Verification error",
  not_applicable: "Not applicable",
};

const demoInvoices = [
  { id: 1042, vendor_name: "Northwind Supplies", invoice_number: "INV-2026-041", invoice_date: "2026-11-18", po_number: "PO-8872", grand_total: 12540, currency: "INR", is_invoice: true, processing_status: "awaiting_po", created_at: "2026-11-19T10:24:00Z" },
  { id: 1041, vendor_name: "Contoso Logistics", invoice_number: "INV-9918", invoice_date: "2026-11-16", po_number: "PO-8849", grand_total: 8420.5, currency: "INR", is_invoice: true, processing_status: "awaiting_po", created_at: "2026-11-17T08:15:00Z" },
  { id: 1040, vendor_name: "Fabrikam Office", invoice_number: "INV-4412", invoice_date: "2026-11-12", po_number: "", grand_total: 2190, currency: "INR", is_invoice: true, processing_status: "not_applicable", created_at: "2026-11-13T14:02:00Z" },
  { id: 1039, vendor_name: "Adatum Services", invoice_number: "", invoice_date: "2026-11-10", po_number: "PO-8791", grand_total: 0, currency: "INR", is_invoice: false, processing_status: "not_applicable", created_at: "2026-11-11T09:45:00Z" },
];

function processingStatusLabel(status) {
  if (!status) return "Status unavailable";
  return Object.hasOwn(processingStatusLabels, status)
    ? processingStatusLabels[status]
    : "Unrecognized status";
}

function processingStatusClass(status) {
  return Object.hasOwn(processingStatusLabels, status) ? status : "unknown";
}

function icon(name) {
  const icons = {
    grid: '<svg viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>',
    chart: '<svg viewBox="0 0 24 24"><path d="M4 20V10m5 10V4m6 16v-7m5 7H2"/></svg>',
  };
  return icons[name];
}

function shell(content) {
  app.innerHTML = `
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark">ID</div>
        <div><strong>Invoice Desk</strong><span>Operations</span></div>
      </div>
      <nav>
        <a href="#invoices" data-route="invoices">${icon("grid")}<span>Invoices</span></a>
        <a href="#reports" data-route="reports">${icon("chart")}<span>Reports</span></a>
      </nav>
      <div class="sidebar-footer"><span class="status-dot"></span> API connected</div>
    </aside>
    <main>${content}</main>
  `;
  const route = location.hash.slice(1) || "invoices";
  document.querySelector(`[data-route="${route}"]`)?.classList.add("active");
}

function formatMoney(value, currency = "") {
  const amount = Number(value);
  if (!Number.isFinite(amount)) return "—";
  return new Intl.NumberFormat("en-IN", {
    style: "currency",
    currency: currency || "INR",
    maximumFractionDigits: 2,
  }).format(amount);
}

function formatDate(value) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("en-IN", {
    day: "2-digit",
    month: "short",
    year: "numeric",
  }).format(new Date(`${value}T00:00:00`));
}

function invoicesPage() {
  shell(`
    <header class="topbar">
      <div><p class="eyebrow">Overview</p><h1>Invoices</h1></div>
      <button class="icon-button" id="refresh-invoices" aria-label="Refresh invoices">↻</button>
    </header>
    <section class="content">
      <div class="filters">
        <label>Vendor<input id="vendor_name" type="search" placeholder="Search vendor"></label>
        <label>Invoice no.<input id="invoice_number" type="search" placeholder="Invoice number"></label>
        <label>PO no.<input id="po_number" type="search" placeholder="Purchase order"></label>
        <label>Processing status
          <select id="processing_status">
            <option value="">All statuses</option>
            <option value="awaiting_po">Awaiting PO</option>
            <option value="processing">Verifying PO</option>
            <option value="verified">Verified</option>
            <option value="mismatch">PO mismatch</option>
            <option value="needs_review">Needs review</option>
            <option value="error">Verification error</option>
            <option value="not_applicable">Not applicable</option>
          </select>
        </label>
        <label>From<input id="from_date" type="date"></label>
        <label>To<input id="to_date" type="date"></label>
        <button id="apply-filters" class="primary">Filter</button>
      </div>
      <div id="invoice-summary" class="summary-grid"></div>
      <div class="table-card">
        <div id="table-status" class="table-status">Loading invoices…</div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Invoice</th><th>Vendor</th><th>Invoice date</th><th>PO number</th><th>Amount</th><th>Processing status</th></tr></thead>
            <tbody id="invoice-table"></tbody>
          </table>
        </div>
        <div class="pagination">
          <span id="page-info"></span>
          <div><button id="prev-page" class="secondary">Previous</button><button id="next-page" class="secondary">Next</button></div>
        </div>
      </div>
    </section>
    <dialog id="invoice-dialog" class="detail-dialog">
      <div class="dialog-header">
        <div><p class="eyebrow">Invoice record</p><h2 id="detail-title">Invoice details</h2></div>
        <button id="close-invoice-dialog" class="dialog-close" aria-label="Close details">×</button>
      </div>
      <div id="invoice-detail-content"></div>
    </dialog>
  `);

  let state = { page: 1, limit: 20, usingDemo: false };

  async function load() {
    const status = document.querySelector("#table-status");
    status.textContent = "Loading invoices…";
    status.className = "table-status";
    const filters = {
      vendor_name: document.querySelector("#vendor_name").value,
      invoice_number: document.querySelector("#invoice_number").value,
      po_number: document.querySelector("#po_number").value,
      processing_status: document.querySelector("#processing_status").value,
      from_date: document.querySelector("#from_date").value,
      to_date: document.querySelector("#to_date").value,
      page: state.page,
      limit: state.limit,
    };

    try {
      const data = await listInvoices(filters);
      state.usingDemo = false;
      render(data);
    } catch (error) {
      state.usingDemo = true;
      const demoRows = demoInvoices.filter((invoice) =>
        !filters.processing_status || invoice.processing_status === filters.processing_status);
      render({ total: demoRows.length, page: 1, limit: 20, invoices: demoRows }, error.message);
    }
  }

  function render(data, demoReason) {
    const tbody = document.querySelector("#invoice-table");
    const status = document.querySelector("#table-status");
    tbody.innerHTML = data.invoices.length
      ? data.invoices.map((invoice) => `
          <tr>
            <td><button class="invoice-link" data-invoice-id="${escapeHtml(invoice.id)}">${escapeHtml(invoice.invoice_number || `#${invoice.id}`)}</button><small>ID ${escapeHtml(invoice.id)}</small></td>
            <td>${escapeHtml(invoice.vendor_name || "—")}</td>
            <td>${formatDate(invoice.invoice_date)}</td>
            <td>${escapeHtml(invoice.po_number || "—")}</td>
            <td class="amount">${formatMoney(invoice.grand_total, invoice.currency)}</td>
            <td><span class="badge ${processingStatusClass(invoice.processing_status)}">${processingStatusLabel(invoice.processing_status)}</span></td>
          </tr>`).join("")
      : `<tr><td colspan="6" class="empty">No invoices match these filters.</td></tr>`;

    const total = data.total ?? data.invoices.length;
    document.querySelector("#invoice-summary").innerHTML = `
      <article><span>Total invoices</span><strong>${total}</strong></article>
      <article><span>PO verified</span><strong>${data.invoices.filter((row) => row.processing_status === "verified").length}</strong></article>
      <article><span>Awaiting PO</span><strong>${data.invoices.filter((row) => row.processing_status === "awaiting_po").length}</strong></article>`;
    document.querySelector("#page-info").textContent = `Page ${data.page} · ${total} records`;
    document.querySelector("#prev-page").disabled = state.page <= 1;
    document.querySelector("#next-page").disabled = state.page * state.limit >= total;

    if (demoReason) {
      status.textContent = `Demo data shown — ${demoReason}`;
      status.className = "table-status warning";
    } else {
      status.textContent = `${total} invoice${total === 1 ? "" : "s"} found`;
      status.className = "table-status";
    }

    tbody.querySelectorAll(".invoice-link").forEach((button) => {
      button.addEventListener("click", () => showInvoiceDetails(button.dataset.invoiceId));
    });
  }

  const dialog = document.querySelector("#invoice-dialog");
  document.querySelector("#close-invoice-dialog").addEventListener("click", () => dialog.close());
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) dialog.close();
  });

  async function showInvoiceDetails(id) {
    const content = document.querySelector("#invoice-detail-content");
    content.innerHTML = '<p class="detail-loading">Loading invoice and email metadata…</p>';
    dialog.showModal();
    try {
      const details = await getInvoice(id);
      renderInvoiceDetails(details);
    } catch (error) {
      content.innerHTML = `<p class="detail-error">${escapeHtml(error.message)}</p>`;
    }
  }

  document.querySelector("#apply-filters").addEventListener("click", () => {
    state.page = 1;
    load();
  });
  document.querySelector("#refresh-invoices").addEventListener("click", load);
  document.querySelector("#prev-page").addEventListener("click", () => { state.page -= 1; load(); });
  document.querySelector("#next-page").addEventListener("click", () => { state.page += 1; load(); });
  load();
}

function reportsPage() {
  shell(`
    <header class="topbar">
      <div><p class="eyebrow">Documents</p><h1>Parse PO file</h1></div>
    </header>
    <section class="content">
      <div class="report-card">
        <div class="report-heading">
          <div class="report-icon">${icon("chart")}</div>
          <div><h2>CSV or Excel parser</h2><p>Parse and preview locally; parsed PO rows are sent to the API only when you generate the report.</p></div>
        </div>
        <div class="parse-step-heading">
          <span class="step-number">1</span>
          <div><h2>Upload PO file</h2><p>Select a .csv or .xlsx file; the first non-empty row is treated as the header.</p></div>
        </div>
        <form id="workbook-form">
          <div class="upload-zone" id="upload-zone">
            <input id="workbook-file" name="file" type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" required>
            <label for="workbook-file">
              <span class="upload-symbol">↑</span>
              <strong id="workbook-file-label">Choose a CSV or Excel file</strong>
              <small>.csv or .xlsx</small>
            </label>
          </div>
          <div id="workbook-message" class="notice" role="status" hidden></div>
          <button id="parse-workbook" class="primary generate" type="submit" disabled>Parse file</button>
        </form>
        <section id="workbook-result" class="workbook-result" aria-live="polite" hidden>
          <div class="parse-step-heading">
            <span class="step-number">2</span>
            <div><h2>File parsed</h2><p id="workbook-summary"></p></div>
          </div>
          <div class="parse-step-heading">
            <span class="step-number">3</span>
            <div><h2>Parsed sheet data</h2><p id="sheet-summary"></p></div>
          </div>
          <div id="sheet-tabs" class="sheet-tabs" role="tablist" aria-label="Workbook sheets"></div>
          <div id="sheet-preview"></div>
          <div class="parse-step-heading">
            <span class="step-number">4</span>
            <div><h2>Generate report</h2><p>Match PO number, vendor, currency, and exact totals against stored invoices.</p></div>
          </div>
          <div id="report-message" class="notice" role="status" hidden></div>
          <button id="generate-report" class="primary generate" type="button" disabled>Generate and download Excel</button>
        </section>
      </div>
    </section>
  `);

  const fileInput = document.querySelector("#workbook-file");
  const fileLabel = document.querySelector("#workbook-file-label");
  const parseButton = document.querySelector("#parse-workbook");
  const message = document.querySelector("#workbook-message");
  const result = document.querySelector("#workbook-result");
  const reportButton = document.querySelector("#generate-report");
  const reportMessage = document.querySelector("#report-message");
  const previewLimit = 50;
  let parsedSheets = [];

  fileInput.addEventListener("change", () => {
    const file = fileInput.files[0];
    fileLabel.textContent = file?.name || "Choose a CSV or Excel file";
    result.hidden = true;
    message.hidden = true;
    reportMessage.hidden = true;
    reportButton.disabled = true;
    parsedSheets = [];
    const extension = file?.name.split(".").pop().toLowerCase();
    parseButton.disabled = !file || !["csv", "xlsx"].includes(extension);
    if (file && !parseButton.disabled) return;
    if (file) showMessage(message, "Please choose a .csv or .xlsx file.", "error");
  });

  document.querySelector("#workbook-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = fileInput.files[0];
    const extension = file?.name.split(".").pop().toLowerCase();
    if (!file || !["csv", "xlsx"].includes(extension)) {
      showMessage(message, "Choose a .csv or .xlsx file first.", "error");
      return;
    }

    parseButton.disabled = true;
    parseButton.textContent = "Parsing file…";
    message.hidden = true;
    result.hidden = true;

    try {
      const sourceSheets = await parseUploadedFile(file, extension);
      if (!Array.isArray(sourceSheets) || sourceSheets.length === 0) {
        throw new Error("The file does not contain any readable data.");
      }
      parsedSheets = sourceSheets.map(({ sheet, data }) => normalizeSheet(sheet, data));
      const rowCount = parsedSheets.reduce((total, sheet) => total + sheet.rows.length, 0);
      renderWorkbookPreview(parsedSheets, file.name, previewLimit);
      reportButton.disabled = rowCount === 0;
      showMessage(
        message,
        `${rowCount} data row${rowCount === 1 ? "" : "s"} normalized across ${parsedSheets.length} sheet${parsedSheets.length === 1 ? "" : "s"}.`,
        "success",
      );
    } catch (error) {
      showMessage(message, `Could not parse file: ${error.message}`, "error");
    } finally {
      parseButton.disabled = false;
      parseButton.textContent = "Parse file";
    }
  });

  reportButton.addEventListener("click", async () => {
    const file = fileInput.files[0];
    if (!file || parsedSheets.length === 0) return;

    reportButton.disabled = true;
    reportButton.textContent = "Generating report…";
    reportMessage.hidden = true;
    try {
      const { blob, filename } = await generateReport({ fileName: file.name, sheets: parsedSheets });
      const url = URL.createObjectURL(blob);
      const download = document.createElement("a");
      download.href = url;
      download.download = filename;
      download.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      showMessage(reportMessage, "Report generated and downloaded. Any matched invoices were updated.", "success");
    } catch (error) {
      showMessage(reportMessage, `Could not generate report: ${error.message}`, "error");
    } finally {
      reportButton.disabled = false;
      reportButton.textContent = "Generate and download Excel";
    }
  });
}

async function parseUploadedFile(file, extension) {
  if (extension === "xlsx") return readExcelFile(file);
  if (extension !== "csv") throw new Error("Only CSV and XLSX files are supported.");

  const parsed = await new Promise((resolve, reject) => {
    Papa.parse(file, {
      complete: resolve,
      error: reject,
    });
  });
  if (parsed.errors.length > 0) {
    throw new Error(parsed.errors[0].message);
  }
  const sheetName = file.name.replace(/\.csv$/i, "") || "CSV";
  return [{ sheet: sheetName, data: parsed.data }];
}

function normalizeSheet(sheetName, rawRows) {
  const rows = Array.isArray(rawRows) ? rawRows : [];
  const headerIndex = rows.findIndex((row) =>
    Array.isArray(row) && row.some((value) => formatSpreadsheetValue(value).trim() !== ""));
  if (headerIndex < 0) return { name: sheetName, header_row: 1, columns: [], rows: [] };

  const headerRow = rows[headerIndex];
  const columnCount = rows.reduce(
    (max, row) => Math.max(max, Array.isArray(row) ? row.length : 0),
    headerRow.length,
  );
  const usedKeys = new Set();
  const columns = Array.from({ length: columnCount }, (_, index) => {
    const label = formatSpreadsheetValue(headerRow[index]).trim() || `Column ${index + 1}`;
    const normalizedLabel = label.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_|_$/g, "")
      || `column_${index + 1}`;
    const aliases = {
      amount: "po_amount",
      currency_code: "currency",
      po: "po_number",
      po_total: "po_amount",
      purchase_order: "po_number",
      purchase_order_number: "po_number",
      supplier: "vendor",
      supplier_name: "vendor",
      total_amount: "po_amount",
      vendor_name: "vendor",
    };
    const baseKey = aliases[normalizedLabel] || normalizedLabel;
    let key = baseKey;
    let suffix = 2;
    while (usedKeys.has(key)) key = `${baseKey}_${suffix++}`;
    usedKeys.add(key);
    return { key, label };
  });

  const dataRows = rows.slice(headerIndex + 1)
    .map((cells, index) => ({
      source_row: headerIndex + index + 2,
      cells: Array.isArray(cells) ? cells : [],
    }))
    .filter(({ cells }) => cells.some((value) => formatSpreadsheetValue(value).trim() !== ""));
  const normalizedRows = dataRows.map(({ source_row, cells }) => ({
    source_row,
    values: Object.fromEntries(columns.map(({ key }, index) => [
      key,
      formatSpreadsheetValue(cells[index]).trim(),
    ])),
  }));
  return { name: sheetName, header_row: headerIndex + 1, columns, rows: normalizedRows };
}

function renderWorkbookPreview(sheets, fileName, previewLimit) {
  const result = document.querySelector("#workbook-result");
  const tabs = document.querySelector("#sheet-tabs");
  const preview = document.querySelector("#sheet-preview");
  const totalRows = sheets.reduce((total, sheet) => total + sheet.rows.length, 0);
  document.querySelector("#workbook-summary").textContent =
    `${fileName} · ${sheets.length} sheet${sheets.length === 1 ? "" : "s"} · ${totalRows} normalized data row${totalRows === 1 ? "" : "s"}`;
  result.hidden = false;

  function showSheet(index) {
    const sheet = sheets[index];
    const previewRows = sheet.rows.slice(0, previewLimit);
    tabs.querySelectorAll("[role='tab']").forEach((tab, tabIndex) => {
      tab.classList.toggle("active", tabIndex === index);
      tab.setAttribute("aria-selected", String(tabIndex === index));
    });
    document.querySelector("#sheet-summary").textContent =
      `${sheet.name} · ${sheet.rows.length} data row${sheet.rows.length === 1 ? "" : "s"} · ` +
      `${sheet.columns.length} column${sheet.columns.length === 1 ? "" : "s"} · ` +
      `${Math.min(sheet.rows.length, previewLimit)} of ${sheet.rows.length} rows shown`;

    if (sheet.columns.length === 0) {
      preview.innerHTML = '<p class="detail-empty">This sheet has no data.</p>';
      return;
    }

    const columns = sheet.columns.map(({ label }) =>
      `<th scope="col">${escapeHtml(label)}</th>`).join("");
    const body = previewRows.length
      ? previewRows.map((row) => `
      <tr>
        <th scope="row">${row.source_row}</th>
        ${sheet.columns.map(({ key }) =>
          `<td>${escapeHtml(row.values[key])}</td>`).join("")}
      </tr>`).join("")
      : `<tr><td class="empty" colspan="${sheet.columns.length + 1}">No data rows after the header.</td></tr>`;
    preview.innerHTML = `
      <div class="workbook-table-wrap">
        <table class="workbook-table">
          <thead><tr><th scope="col">Row</th>${columns}</tr></thead>
          <tbody>${body}</tbody>
        </table>
      </div>`;
  }

  tabs.innerHTML = sheets.map((sheet, index) => `
    <button class="sheet-tab${index === 0 ? " active" : ""}" type="button" role="tab"
      aria-selected="${index === 0}" data-sheet-index="${index}">${escapeHtml(sheet.name)}</button>
  `).join("");
  tabs.querySelectorAll("[role='tab']").forEach((tab) => {
    tab.addEventListener("click", () => showSheet(Number(tab.dataset.sheetIndex)));
  });
  showSheet(0);
}

function formatSpreadsheetValue(value) {
  if (value === null || value === undefined) return "";
  if (value instanceof Date && !Number.isNaN(value.getTime())) {
    return value.toLocaleString();
  }
  return String(value);
}

function showMessage(element, text, type) {
  element.textContent = text;
  element.className = `notice ${type}`;
  element.hidden = false;
}

function renderInvoiceDetails(details) {
  const invoice = details.invoice || {};
  const email = details.email || {};
  const extraction = details.extraction || {};
  const emailBody = typeof email.body === "string"
    ? email.body
    : email.body?.text || email.body?.html || "";
  const value = (item) => item === null || item === undefined || item === "" ? "—" : String(item);
  const detailItems = (items) => items.map(([label, item]) => `
    <div class="detail-item"><span>${escapeHtml(label)}</span><strong>${escapeHtml(value(item))}</strong></div>
  `).join("");
  const lineItems = Array.isArray(details.line_items) ? details.line_items : [];

  document.querySelector("#detail-title").textContent =
    invoice.invoice_number || `Invoice #${invoice.id || ""}`;
  document.querySelector("#invoice-detail-content").innerHTML = `
    <section class="detail-section">
      <h3>Invoice information</h3>
      <div class="detail-grid">${detailItems([
        ["Vendor", invoice.vendor_name],
        ["Invoice number", invoice.invoice_number],
        ["Invoice date", invoice.invoice_date],
        ["Due date", invoice.due_date],
        ["PO number", invoice.po_number],
        ["Processing status", processingStatusLabel(invoice.processing_status)],
        ["Invoice classification", invoice.is_invoice === undefined ? "—" : invoice.is_invoice ? "Invoice" : "Not an invoice"],
        ["Subtotal", invoice.subtotal === undefined ? "—" : formatMoney(invoice.subtotal, invoice.currency)],
        ["Tax", invoice.tax_amount === undefined ? "—" : formatMoney(invoice.tax_amount, invoice.currency)],
        ["Total", invoice.grand_total === undefined ? "—" : formatMoney(invoice.grand_total, invoice.currency)],
        ["Currency", invoice.currency],
        ["Created", formatDateTime(invoice.created_at)],
        ["Review reason", invoice.reason],
      ])}</div>
    </section>
    <section class="detail-section">
      <h3>Source email</h3>
      <div class="detail-grid">${detailItems([
        ["From", email.sender],
        ["To", email.recipient],
        ["Subject", email.subject],
        ["Received", formatDateTime(email.email_date)],
        ["Gmail ID", email.gmail_id],
        ["Thread ID", email.thread_id],
        ["Message ID", email.message_id],
      ])}</div>
      ${email.gmail_url ? `<a class="email-link" href="${escapeHtml(email.gmail_url)}" target="_blank" rel="noopener noreferrer">Open in Gmail ↗</a>` : ""}
      ${emailBody ? `<div class="email-body"><span>Email body</span><pre>${escapeHtml(emailBody)}</pre></div>` : ""}
    </section>
    <section class="detail-section">
      <h3>Extraction metadata</h3>
      <div class="detail-grid">${detailItems([
        ["Channel", extraction.channel],
        ["Model", extraction.model],
        ["Completed", extraction.done === undefined ? "—" : extraction.done ? "Yes" : "No"],
        ["Completion reason", extraction.done_reason],
        ["Response count", extraction.response_count],
        ["Extracted at", formatDateTime(extraction.created_at)],
      ])}</div>
    </section>
    <section class="detail-section">
      <h3>Line items <span class="section-count">${lineItems.length}</span></h3>
      ${lineItems.length ? `<div class="detail-table-wrap"><table class="detail-table">
        <thead><tr><th>Description</th><th>Qty</th><th>Unit</th><th>Unit price</th><th>Total</th></tr></thead>
        <tbody>${lineItems.map((item) => `<tr>
          <td>${escapeHtml(value(item.description))}</td>
          <td>${escapeHtml(value(item.quantity))}</td>
          <td>${escapeHtml(value(item.unit))}</td>
          <td>${escapeHtml(item.unit_price === undefined ? "—" : formatMoney(item.unit_price, invoice.currency))}</td>
          <td>${escapeHtml(item.line_total === undefined ? "—" : formatMoney(item.line_total, invoice.currency))}</td>
        </tr>`).join("")}</tbody>
      </table></div>` : '<p class="detail-empty">No line items returned.</p>'}
    </section>
  `;
}

function formatDateTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat("en-IN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (char) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;",
  })[char]);
}

function route() {
  const routeName = location.hash.slice(1) || "invoices";
  if (routeName === "reports") reportsPage();
  else invoicesPage();
}

window.addEventListener("hashchange", route);
route();
