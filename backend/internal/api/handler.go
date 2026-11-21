package api

import (
	"log/slog"
	"net/http"

	"invoice-tracker/backend/internal/invoices"
	"invoice-tracker/backend/internal/pdfimages"
)

func NewHandler(
	invoiceRepository invoices.Repository,
	pdfConverter pdfimages.Converter,
	allowedOrigin string,
	logger *slog.Logger,
) http.Handler {
	mux := http.NewServeMux()
	invoiceHandler := invoices.NewHandler(invoiceRepository, logger)
	mux.Handle("/api/v1/invoice.create", invoiceHandler)
	mux.Handle("/api/v1/invoice.bulk-create", invoiceHandler)
	mux.Handle("/api/v1/invoice.list", invoiceHandler)
	mux.Handle("/api/v1/invoice.get", invoiceHandler)
	mux.Handle("/api/v1/convert", pdfimages.NewHandler(pdfConverter, logger))
	return cors(mux, allowedOrigin)
}

func cors(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, ngrok-skip-browser-warning")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
