export const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL || "http://localhost:8081/api/v1";

const apiHeaders = new URL(API_BASE_URL).hostname.endsWith(".ngrok-free.app")
  ? { "ngrok-skip-browser-warning": "true" }
  : {};

function apiFetch(url, options = {}) {
  return fetch(url, {
    ...options,
    headers: { ...apiHeaders, ...options.headers },
  });
}

async function parseError(response) {
  try {
    const body = await response.json();
    return body.err_msg || body.message || `Request failed (${response.status})`;
  } catch {
    return `Request failed (${response.status})`;
  }
}

export async function listInvoices(filters = {}) {
  const params = new URLSearchParams();
  Object.entries(filters).forEach(([key, value]) => {
    if (value !== "" && value !== null && value !== undefined) {
      params.set(key, String(value));
    }
  });

  const response = await apiFetch(`${API_BASE_URL}/invoice.list?${params}`);
  if (!response.ok) throw new Error(await parseError(response));
  return response.json();
}

export async function getInvoice(id) {
  const params = new URLSearchParams({ id: String(id) });
  const response = await apiFetch(`${API_BASE_URL}/invoice.get?${params}`);
  if (!response.ok) throw new Error(await parseError(response));
  return response.json();
}

export async function generateReport({ fileName, sheets }) {
  const response = await apiFetch(`${API_BASE_URL}/report.gen`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ file_name: fileName, sheets }),
  });

  if (!response.ok) throw new Error(await parseError(response));

  const blob = await response.blob();
  if (!blob.size) throw new Error("The server returned an empty report.");

  const disposition = response.headers.get("Content-Disposition") || "";
  const match = disposition.match(/filename="?([^";]+)"?/i);
  const fallbackName = `purchase-order-reconciliation-${new Date().toISOString().slice(0, 10)}.xlsx`;
  return { blob, filename: match?.[1] || fallbackName };
}
