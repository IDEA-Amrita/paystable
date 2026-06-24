package adminapi

import (
	"net/http"
	"time"
)

func (h *Handler) overviewStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var activeHolds, pendingDeliveries, exhaustedDeliveries, rejectedWebhooks int

	h.db.QueryRowContext(ctx, `SELECT count(*) FROM holds WHERE status IN ('PENDING','VERIFYING')`).Scan(&activeHolds)                      //nolint:errcheck
	h.db.QueryRowContext(ctx, `SELECT count(*) FROM outbox WHERE status='pending'`).Scan(&pendingDeliveries)                                //nolint:errcheck
	h.db.QueryRowContext(ctx, `SELECT count(*) FROM outbox WHERE status='exhausted'`).Scan(&exhaustedDeliveries)                            //nolint:errcheck
	h.db.QueryRowContext(ctx, `SELECT count(*) FROM webhooks_rejected WHERE received_at > now()-interval '1 hour'`).Scan(&rejectedWebhooks) //nolint:errcheck

	status := func(n, warn int) string {
		if n >= warn {
			return "warning"
		}
		return "normal"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"active_holds":         map[string]any{"value": activeHolds, "status": status(activeHolds, 20)},
		"pending_deliveries":   map[string]any{"value": pendingDeliveries, "status": status(pendingDeliveries, 50)},
		"exhausted_deliveries": map[string]any{"value": exhaustedDeliveries, "status": status(exhaustedDeliveries, 1)},
		"rejected_webhooks":    map[string]any{"value": rejectedWebhooks, "subtext": "/ 1hr", "status": status(rejectedWebhooks, 5)},
	})
}

func (h *Handler) deliveryStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var deliveredToday, pending, exhausted int
	h.db.QueryRowContext(ctx, `SELECT count(*) FROM outbox WHERE status='delivered' AND delivered_at > now()-interval '24 hours'`).Scan(&deliveredToday) //nolint:errcheck
	h.db.QueryRowContext(ctx, `SELECT count(*) FROM outbox WHERE status='pending'`).Scan(&pending)                                                       //nolint:errcheck
	h.db.QueryRowContext(ctx, `SELECT count(*) FROM outbox WHERE status='exhausted'`).Scan(&exhausted)                                                   //nolint:errcheck

	writeJSON(w, http.StatusOK, map[string]any{
		"delivered_today": deliveredToday,
		"pending":         pending,
		"exhausted":       exhausted,
	})
}

func (h *Handler) deliveries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "exhausted"
	}

	rows, err := h.db.QueryContext(ctx, `
		SELECT o.id, o.txn_id, o.event_type, o.attempts, o.max_attempts,
		       coalesce(last_error,''), last_attempt_at, h.callback_url
		FROM outbox o
		JOIN holds h ON o.txn_id = h.txn_id
		WHERE o.status = $1
		ORDER BY o.last_attempt_at DESC NULLS LAST
		LIMIT 200`, status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() {
		_ = rows.Close()
	}()

	type delivery struct {
		ID            int64      `json:"id"`
		TxnID         string     `json:"txn_id"`
		EventType     string     `json:"event_type"`
		Attempts      int        `json:"attempts"`
		MaxAttempts   int        `json:"max_attempts"`
		LastError     string     `json:"last_error"`
		LastAttemptAt *time.Time `json:"last_attempt_at"`
		CallbackURL   string     `json:"callback_url"`
		Status        string     `json:"status"`
	}

	data := []delivery{}
	for rows.Next() {
		var d delivery
		if err := rows.Scan(&d.ID, &d.TxnID, &d.EventType, &d.Attempts, &d.MaxAttempts,
			&d.LastError, &d.LastAttemptAt, &d.CallbackURL); err != nil {
			continue
		}
		d.Status = status
		data = append(data, d)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data, "total": len(data)})
}
