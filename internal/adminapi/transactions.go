package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
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
	defer func() {
		_ = rows.Close()
	}()

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
		defer func() {
			_ = scRows.Close()
		}()
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
	events := []timelineEvent{}
	events = h.appendHoldCreatedEvent(ctx, events, txnID)
	events = h.appendWebhookEvents(ctx, events, txnID)
	events = h.appendRejectedWebhookEvents(ctx, events, txnID)
	events = h.appendPollEvents(ctx, events, txnID)
	events = h.appendLedgerEvents(ctx, events, txnID)
	events = h.appendCallbackEvents(ctx, events, txnID)

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events
}

func (h *Handler) appendHoldCreatedEvent(ctx context.Context, events []timelineEvent, txnID string) []timelineEvent {
	var gateway, status string
	var amount int64
	var createdAt time.Time
	err := h.db.QueryRowContext(ctx, `
		SELECT gateway, status, amount, created_at
		FROM holds WHERE txn_id = $1`, txnID).
		Scan(&gateway, &status, &amount, &createdAt)
	if err != nil {
		return events
	}

	return append(events, timelineEvent{
		Type:      "hold_created",
		Timestamp: createdAt,
		Source:    "api",
		Detail:    "Hold created",
		Data: map[string]any{
			"gateway": gateway,
			"status":  status,
			"amount":  amount,
		},
	})
}

func (h *Handler) appendWebhookEvents(ctx context.Context, events []timelineEvent, txnID string) []timelineEvent {
	rows, err := h.db.QueryContext(ctx, `
		SELECT gateway, gateway_event_id, event_type, payload, received_at
		FROM webhooks WHERE txn_id = $1 ORDER BY received_at ASC`, txnID)
	if err != nil {
		return events
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var gateway, eventType string
		var gatewayEventID sql.NullString
		var payload json.RawMessage
		var receivedAt time.Time
		if err := rows.Scan(&gateway, &gatewayEventID, &eventType, &payload, &receivedAt); err != nil {
			continue
		}
		data := webhookTimelineData(eventType, gatewayEventID, payload)
		data["gateway"] = gateway
		events = append(events, timelineEvent{
			Type:      "webhook_received",
			Timestamp: receivedAt,
			Source:    "webhook",
			Detail:    eventType + " received",
			Data:      data,
		})
	}
	return events
}

func (h *Handler) appendRejectedWebhookEvents(ctx context.Context, events []timelineEvent, txnID string) []timelineEvent {
	rows, err := h.db.QueryContext(ctx, `
		SELECT gateway, rejection_reason, raw_body, received_at
		FROM webhooks_rejected
		WHERE position(convert_to($1::text, 'UTF8') in raw_body) > 0
		ORDER BY received_at ASC`, txnID)
	if err != nil {
		return events
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var gateway, reason string
		var rawBody []byte
		var receivedAt time.Time
		if err := rows.Scan(&gateway, &reason, &rawBody, &receivedAt); err != nil {
			continue
		}
		params := parseWebhookLikeBody(rawBody)
		if webhookTxnID(params) != txnID {
			continue
		}

		data := map[string]any{
			"gateway":          gateway,
			"rejection_reason": reason,
		}
		if status := params["status"]; status != "" {
			data["status"] = status
			data["gateway_status"] = status
		}
		if eventType := rejectedWebhookEventType(params); eventType != "" {
			data["event_type"] = eventType
		}
		events = append(events, timelineEvent{
			Type:      "webhook_rejected",
			Timestamp: receivedAt,
			Source:    "webhook",
			Detail:    "Rejected webhook: " + reason,
			Data:      data,
		})
	}
	return events
}

func (h *Handler) appendPollEvents(ctx context.Context, events []timelineEvent, txnID string) []timelineEvent {
	rows, err := h.db.QueryContext(ctx, `
		SELECT attempt_number, status, coalesce(gateway_status, ''), gateway_amount,
		       coalesce(error, ''), scheduled_at, started_at, completed_at
		FROM verification_polls
		WHERE txn_id = $1 AND status IN ('completed', 'failed')
		ORDER BY coalesce(completed_at, started_at, scheduled_at) ASC`, txnID)
	if err != nil {
		return events
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var attempt int
		var status, gatewayStatus, errorText string
		var gatewayAmount sql.NullInt64
		var scheduledAt time.Time
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&attempt, &status, &gatewayStatus, &gatewayAmount,
			&errorText, &scheduledAt, &startedAt, &completedAt); err != nil {
			continue
		}

		timestamp := scheduledAt
		if startedAt.Valid {
			timestamp = startedAt.Time
		}
		if completedAt.Valid {
			timestamp = completedAt.Time
		}

		data := map[string]any{"attempt": attempt}
		if gatewayStatus != "" {
			data["gateway_status"] = gatewayStatus
		}
		if gatewayAmount.Valid {
			data["gateway_amount"] = gatewayAmount.Int64
		}
		if errorText != "" {
			data["error"] = errorText
		}

		eventType := "poll_completed"
		detail := "Poll completed"
		if gatewayStatus != "" {
			detail = "Gateway status " + gatewayStatus
		}
		if status == "failed" {
			eventType = "poll_failed"
			detail = "Poll failed"
			if errorText != "" {
				detail += ": " + errorText
			}
		}

		events = append(events, timelineEvent{
			Type:      eventType,
			Timestamp: timestamp,
			Source:    "stabilizer",
			Detail:    detail,
			Attempt:   attempt,
			Data:      data,
		})
	}
	return events
}

func (h *Handler) appendLedgerEvents(ctx context.Context, events []timelineEvent, txnID string) []timelineEvent {
	rows, err := h.db.QueryContext(ctx, `
		SELECT event_type, source, coalesce(from_status,''), coalesce(to_status,''), created_at
		FROM ledger WHERE txn_id = $1 ORDER BY created_at ASC`, txnID)
	if err != nil {
		return events
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var eventType, source, fromStatus, toStatus string
		var createdAt time.Time
		if err := rows.Scan(&eventType, &source, &fromStatus, &toStatus, &createdAt); err != nil {
			continue
		}
		if eventType == "callback_delivered" || eventType == "callback_failed" {
			continue
		}

		e := timelineEvent{
			Type:      eventType,
			Timestamp: createdAt,
			Source:    source,
			Detail:    eventType,
		}
		if toStatus != "" {
			e.Data = map[string]any{"from": fromStatus, "to": toStatus}
			e.Detail = fromStatus + " -> " + toStatus
		}
		events = append(events, e)
	}
	return events
}

func (h *Handler) appendCallbackEvents(ctx context.Context, events []timelineEvent, txnID string) []timelineEvent {
	rows, err := h.db.QueryContext(ctx, `
		SELECT event_type, status, attempts, last_http_status, coalesce(last_error, ''),
		       delivered_at, last_attempt_at, created_at
		FROM outbox
		WHERE txn_id = $1 AND status IN ('delivered', 'exhausted')
		ORDER BY coalesce(delivered_at, last_attempt_at, created_at) ASC`, txnID)
	if err != nil {
		return events
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var callbackEventType, status, errorText string
		var attempts int
		var httpStatus sql.NullInt64
		var deliveredAt, lastAttemptAt sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(&callbackEventType, &status, &attempts, &httpStatus,
			&errorText, &deliveredAt, &lastAttemptAt, &createdAt); err != nil {
			continue
		}

		timestamp := createdAt
		if lastAttemptAt.Valid {
			timestamp = lastAttemptAt.Time
		}
		if deliveredAt.Valid {
			timestamp = deliveredAt.Time
		}

		eventType := "callback_delivered"
		detail := "Callback delivered"
		if status == "exhausted" {
			eventType = "callback_failed"
			detail = "Callback failed"
			if errorText != "" {
				detail += ": " + errorText
			}
		}

		data := map[string]any{
			"callback_event_type": callbackEventType,
			"attempts":            attempts,
		}
		if httpStatus.Valid {
			data["callback_http_status"] = httpStatus.Int64
		}
		if errorText != "" {
			data["error"] = errorText
		}

		events = append(events, timelineEvent{
			Type:      eventType,
			Timestamp: timestamp,
			Source:    "delivery",
			Detail:    detail,
			Data:      data,
		})
	}
	return events
}

func webhookTimelineData(eventType string, gatewayEventID sql.NullString, payload json.RawMessage) map[string]any {
	data := map[string]any{"event_type": eventType}
	if gatewayEventID.Valid && gatewayEventID.String != "" {
		data["gateway_event_id"] = gatewayEventID.String
	}

	params := parseWebhookLikeBody(payload)
	if status := params["status"]; status != "" {
		data["status"] = status
		data["gateway_status"] = status
	}
	if amount := params["amount"]; amount != "" {
		data["gateway_amount"] = amount
	}

	return data
}

func parseWebhookLikeBody(body []byte) map[string]string {
	params := map[string]string{}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return params
	}

	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal(body, &params); err == nil {
			return params
		}
		var generic map[string]any
		if err := json.Unmarshal(body, &generic); err == nil {
			for k, v := range generic {
				if s := stringValue(v); s != "" {
					params[k] = s
				}
			}
		}
		return params
	}

	values, err := url.ParseQuery(trimmed)
	if err != nil {
		return params
	}
	for key, value := range values {
		if len(value) > 0 {
			params[key] = value[0]
		}
	}
	return params
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

func webhookTxnID(params map[string]string) string {
	for _, key := range []string{"txnid", "txn_id", "transaction_id"} {
		if params[key] != "" {
			return params[key]
		}
	}
	return ""
}

func rejectedWebhookEventType(params map[string]string) string {
	if eventType := params["event_type"]; eventType != "" {
		return eventType
	}
	if status := params["status"]; status != "" {
		return "payment." + status
	}
	return ""
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
	defer func() {
		_ = rows.Close()
	}()

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
