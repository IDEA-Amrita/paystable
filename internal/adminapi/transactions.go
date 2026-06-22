package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (h *Handler) transactions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	statusFilter := q.Get("status")
	search := q.Get("search")
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 {
		limit = 25
	}
	offset := (page - 1) * limit

	where := "WHERE 1=1"
	args := []any{}
	n := 1
	if statusFilter != "" && statusFilter != "all" {
		where += " AND status = $" + strconv.Itoa(n)
		args = append(args, statusFilter)
		n++
	}
	if search != "" {
		where += " AND (txn_id ILIKE $" + strconv.Itoa(n) + " OR gateway ILIKE $" + strconv.Itoa(n) + ")"
		args = append(args, "%"+search+"%")
		n++
	}

	var total int
	h.db.QueryRowContext(ctx, "SELECT count(*) FROM holds "+where, args...).Scan(&total) //nolint:errcheck

	rows, err := h.db.QueryContext(ctx, `
		SELECT txn_id, gateway, status, amount, created_at, updated_at
		FROM holds `+where+`
		ORDER BY created_at DESC
		LIMIT $`+strconv.Itoa(n)+` OFFSET $`+strconv.Itoa(n+1),
		append(args, limit, offset)...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type txn struct {
		TxnID             string    `json:"txn_id"`
		Gateway           string    `json:"gateway"`
		Status            string    `json:"status"`
		Amount            int64     `json:"amount"`
		CreatedAt         time.Time `json:"created_at"`
		ResolveDurationMs *int64    `json:"resolve_duration_ms"`
	}

	data := []txn{}
	for rows.Next() {
		var t txn
		var updated time.Time
		if err := rows.Scan(&t.TxnID, &t.Gateway, &t.Status, &t.Amount, &t.CreatedAt, &updated); err != nil {
			continue
		}
		if t.Status != "PENDING" && t.Status != "VERIFYING" {
			ms := updated.Sub(t.CreatedAt).Milliseconds()
			t.ResolveDurationMs = &ms
		}
		data = append(data, t)
	}

	statusCounts := map[string]int{}
	scRows, err := h.db.QueryContext(ctx, "SELECT status, count(*) FROM holds GROUP BY status")
	if err == nil {
		defer scRows.Close()
		for scRows.Next() {
			var s string
			var c int
			if scRows.Scan(&s, &c) == nil {
				statusCounts[s] = c
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data, "total": total, "page": page, "limit": limit, "status_counts": statusCounts,
	})
}

func (h *Handler) transactionDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	txnID := r.PathValue("id")

	var t struct {
		TxnID     string
		Gateway   string
		Status    string
		Amount    int64
		CreatedAt time.Time
		UpdatedAt time.Time
	}
	err := h.db.QueryRowContext(ctx, `
		SELECT txn_id, gateway, status, amount, created_at, updated_at
		FROM holds WHERE txn_id = $1`, txnID).
		Scan(&t.TxnID, &t.Gateway, &t.Status, &t.Amount, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transaction not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	events := h.buildTimeline(ctx, txnID)
	polls := h.buildPolls(ctx, txnID)
	rawWebhook := h.latestWebhookPayload(ctx, txnID)

	var resolveMs *int64
	if t.Status != "PENDING" && t.Status != "VERIFYING" {
		ms := t.UpdatedAt.Sub(t.CreatedAt).Milliseconds()
		resolveMs = &ms
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"txn_id":              t.TxnID,
		"gateway":             t.Gateway,
		"status":              t.Status,
		"amount":              t.Amount,
		"created_at":          t.CreatedAt,
		"resolve_duration_ms": resolveMs,
		"events":              events,
		"polls":               polls,
		"raw_webhook":         rawWebhook,
	})
}

type timelineEvent struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"`
	Detail    string         `json:"detail"`
	Attempt   int            `json:"attempt,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

func (h *Handler) buildTimeline(ctx context.Context, txnID string) []timelineEvent {
	rows, err := h.db.QueryContext(ctx, `
		SELECT event_type, source, coalesce(from_status,''), coalesce(to_status,''), detail, created_at
		FROM ledger WHERE txn_id = $1 ORDER BY created_at ASC`, txnID)
	if err != nil {
		return []timelineEvent{}
	}
	defer rows.Close()

	events := []timelineEvent{}
	for rows.Next() {
		var eventType, source, fromStatus, toStatus string
		var detail json.RawMessage
		var createdAt time.Time
		if err := rows.Scan(&eventType, &source, &fromStatus, &toStatus, &detail, &createdAt); err != nil {
			continue
		}
		e := timelineEvent{Type: eventType, Timestamp: createdAt, Source: source}
		if toStatus != "" {
			e.Data = map[string]any{"from": fromStatus, "to": toStatus}
			e.Detail = fromStatus + " → " + toStatus
		}
		events = append(events, e)
	}
	return events
}

type poll struct {
	Attempt         int       `json:"attempt"`
	Timestamp       time.Time `json:"timestamp"`
	GatewayResponse string    `json:"gateway_response"`
	Amount          int64     `json:"amount"`
	DeltaFromPrev   *int64    `json:"delta_from_prev"`
}

func (h *Handler) buildPolls(ctx context.Context, txnID string) []poll {
	rows, err := h.db.QueryContext(ctx, `
		SELECT attempt_number, coalesce(gateway_status,''), coalesce(gateway_amount,0), completed_at
		FROM verification_polls
		WHERE txn_id = $1 AND status = 'completed'
		ORDER BY completed_at ASC`, txnID)
	if err != nil {
		return []poll{}
	}
	defer rows.Close()

	polls := []poll{}
	var prev *time.Time
	for rows.Next() {
		var p poll
		var gwStatus string
		var completedAt sql.NullTime
		if err := rows.Scan(&p.Attempt, &gwStatus, &p.Amount, &completedAt); err != nil {
			continue
		}
		if completedAt.Valid {
			p.Timestamp = completedAt.Time
			if prev != nil {
				d := p.Timestamp.Sub(*prev).Milliseconds()
				p.DeltaFromPrev = &d
			}
			prev = &completedAt.Time
		}
		if gwStatus == "success" || gwStatus == "captured" || gwStatus == "completed" {
			p.GatewayResponse = "success"
		} else {
			p.GatewayResponse = "failure"
		}
		polls = append(polls, p)
	}
	return polls
}

func (h *Handler) latestWebhookPayload(ctx context.Context, txnID string) json.RawMessage {
	var payload json.RawMessage
	err := h.db.QueryRowContext(ctx, `
		SELECT payload FROM webhooks WHERE txn_id = $1 ORDER BY received_at DESC LIMIT 1`, txnID).Scan(&payload)
	if err != nil {
		return nil
	}
	return payload
}
