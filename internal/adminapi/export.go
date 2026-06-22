package adminapi

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"time"
)

// ExportLedger serves GET /api/v1/admin/export/ledger?format=csv|json&txn_id=<optional>
func (h *Handler) ExportLedger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	txnFilter := r.URL.Query().Get("txn_id")

	query := `SELECT txn_id, event_type, source, coalesce(from_status,''), coalesce(to_status,''), created_at FROM ledger`
	args := []any{}
	if txnFilter != "" {
		query += " WHERE txn_id = $1"
		args = append(args, txnFilter)
	}
	query += " ORDER BY created_at ASC"

	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type entry struct {
		TxnID      string    `json:"txn_id"`
		EventType  string    `json:"event_type"`
		Source     string    `json:"source"`
		FromStatus string    `json:"from_status"`
		ToStatus   string    `json:"to_status"`
		CreatedAt  time.Time `json:"created_at"`
	}

	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.TxnID, &e.EventType, &e.Source, &e.FromStatus, &e.ToStatus, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="ledger-export.csv"`)
		cw := csv.NewWriter(w)
		cw.Write([]string{"txn_id", "event_type", "source", "from_status", "to_status", "created_at"}) //nolint:errcheck
		for _, e := range entries {
			cw.Write([]string{e.TxnID, e.EventType, e.Source, e.FromStatus, e.ToStatus, e.CreatedAt.Format(time.RFC3339)}) //nolint:errcheck
		}
		cw.Flush()
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="ledger-export.json"`)
	json.NewEncoder(w).Encode(entries) //nolint:errcheck
}
