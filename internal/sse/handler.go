package sse

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	txnID := r.PathValue("txn_id")
	token := r.URL.Query().Get("token")
	if txnID == "" || token == "" {
		http.Error(w, "missing txn_id or token", http.StatusBadRequest)
		return
	}

	// Validate token
	var exists bool
	err := h.db.QueryRowContext(r.Context(),
		`SELECT true FROM holds WHERE txn_id=$1 AND read_token=$2`, txnID, token).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial status
	var currentStatus string
	h.db.QueryRowContext(r.Context(), `SELECT status FROM holds WHERE txn_id=$1`, txnID).Scan(&currentStatus) //nolint:errcheck
	fmt.Fprintf(w, "event: status_change\ndata: {\"status\":\"%s\",\"at\":\"%s\"}\n\n",
		currentStatus, time.Now().UTC().Format(time.RFC3339))
	flusher.Flush()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			var newStatus string
			if err := h.db.QueryRowContext(r.Context(),
				`SELECT status FROM holds WHERE txn_id=$1`, txnID).Scan(&newStatus); err != nil {
				slog.Warn("sse: status query", "error", err)
				return
			}
			if newStatus != currentStatus {
				currentStatus = newStatus
				fmt.Fprintf(w, "event: status_change\ndata: {\"status\":\"%s\",\"at\":\"%s\"}\n\n",
					currentStatus, time.Now().UTC().Format(time.RFC3339))
				flusher.Flush()
			}
			// Send heartbeat comment to keep connection alive
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
			// Terminal states: close the stream
			if currentStatus == "CONFIRMED" || currentStatus == "FAILED" ||
				currentStatus == "REFUNDED" || currentStatus == "INDETERMINATE" ||
				currentStatus == "MISMATCH" {
				return
			}
		}
	}
}
