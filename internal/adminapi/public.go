package adminapi

import (
	"net/http"
)

// PublicTimeline serves GET /api/v1/transactions/:id/timeline
// Auth: ?token=<read_token> (validated against holds.read_token)
func (h *Handler) PublicTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	txnID := r.PathValue("id")
	token := r.URL.Query().Get("token")

	if txnID == "" || token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing txn_id or token"})
		return
	}

	// Validate token against holds table
	var exists bool
	err := h.db.QueryRowContext(ctx,
		`SELECT true FROM holds WHERE txn_id=$1 AND read_token=$2`, txnID, token).Scan(&exists)
	if err != nil || !exists {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	events := h.buildTimeline(ctx, txnID)
	writeJSON(w, http.StatusOK, map[string]any{"txn_id": txnID, "events": events})
}
