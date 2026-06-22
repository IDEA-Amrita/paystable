package adminapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// A mismatch is a transaction whose first webhook claimed failure but which
// paystable ultimately CONFIRMED (or claimed success but ended FAILED).
const mismatchQuery = `
	SELECT h.txn_id, h.gateway, w.event_type, h.status, h.created_at, h.updated_at
	FROM holds h
	JOIN LATERAL (
		SELECT event_type FROM webhooks
		WHERE txn_id = h.txn_id ORDER BY received_at ASC LIMIT 1
	) w ON true
	WHERE (w.event_type LIKE '%fail%' AND h.status = 'CONFIRMED')
	   OR (w.event_type LIKE '%success%' AND h.status = 'FAILED')`

func (h *Handler) mismatches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 25
	offset := (page - 1) * limit

	rows, err := h.db.QueryContext(ctx, mismatchQuery+` ORDER BY h.updated_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type mismatch struct {
		TxnID          string    `json:"txn_id"`
		Gateway        string    `json:"gateway"`
		WebhookClaimed string    `json:"webhook_claimed"`
		VerifiedTruth  string    `json:"verified_truth"`
		DetectedAt     time.Time `json:"detected_at"`
		TimeSavedMs    int64     `json:"time_saved_ms"`
	}

	data := []mismatch{}
	for rows.Next() {
		var m mismatch
		var created, updated time.Time
		if err := rows.Scan(&m.TxnID, &m.Gateway, &m.WebhookClaimed, &m.VerifiedTruth, &created, &updated); err != nil {
			continue
		}
		m.TimeSavedMs = updated.Sub(created).Milliseconds()
		m.DetectedAt = updated
		data = append(data, m)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": data, "total": len(data), "page": page, "limit": limit})
}

func (h *Handler) mismatchStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var last7 int
	h.db.QueryRowContext(ctx,
		`SELECT count(*) FROM (`+mismatchQuery+` AND h.updated_at > now()-interval '7 days') x`).Scan(&last7) //nolint:errcheck

	writeJSON(w, http.StatusOK, map[string]any{"last_7_days": last7})
}

func (h *Handler) config(w http.ResponseWriter, r *http.Request) {
	isSet := func(v string) bool { return v != "" }
	secret := func(key, val string) map[string]any {
		return map[string]any{"key": key, "value": nil, "is_secret": true, "is_set": isSet(val)}
	}
	plain := func(key, val string) map[string]any {
		return map[string]any{"key": key, "value": val, "is_secret": false, "is_set": true}
	}
	writeJSON(w, http.StatusOK, []map[string]any{
		plain("GATEWAY", h.cfg.Gateway),
		plain("STABILIZATION_N", strconv.Itoa(h.cfg.StabilizationN)),
		plain("MAX_BACKOFF_S", strconv.Itoa(h.cfg.MaxBackoffS)),
		plain("HOLD_MAX_TTL_S", strconv.Itoa(h.cfg.HoldMaxTTLS)),
		plain("LOG_LEVEL", h.cfg.LogLevel),
		secret("WEBHOOK_SECRET", h.cfg.WebhookSecret),
		secret("GATEWAY_API_KEY", h.cfg.GatewayAPIKey),
		secret("MERCHANT_CALLBACK_SECRET", h.cfg.MerchantCallbackSecret),
	})
}

func (h *Handler) rotationStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var lastRotated *time.Time
	var windowEnd *time.Time
	var active bool

	h.db.QueryRowContext(ctx, `
		SELECT created_at, rotation_window_end,
		       (rotation_window_end IS NOT NULL AND rotation_window_end > now())
		FROM gateway_secrets
		WHERE gateway = 'payu'
		ORDER BY created_at DESC LIMIT 1`).Scan(&lastRotated, &windowEnd, &active) //nolint:errcheck

	resp := map[string]any{
		"is_active":       active,
		"last_rotated_at": lastRotated,
		"window_ends_at":  windowEnd,
	}
	if active && windowEnd != nil {
		resp["hours_remaining"] = int(time.Until(*windowEnd).Hours())
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) replayDelivery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	res, err := h.db.ExecContext(ctx, `
		UPDATE outbox SET status='pending', next_attempt_at=now(), attempts=0, last_error=NULL
		WHERE id = $1 AND status = 'exhausted'`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no exhausted delivery with that id"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Delivery queued for replay"})
}

func (h *Handler) rotateSecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Gateway     string `json:"gateway"`
		NewSecret   string `json:"new_secret"`
		WindowHours int    `json:"window_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.Gateway == "" || req.NewSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gateway and new_secret required"})
		return
	}
	if req.WindowHours <= 0 {
		req.WindowHours = 24
	}
	// Encrypt with AES-256-GCM — for now store as plain bytes (TODO: real encryption)
	windowEnd := time.Now().Add(time.Duration(req.WindowHours) * time.Hour)
	_, err := h.db.ExecContext(ctx,
		`INSERT INTO gateway_secrets (gateway, secret_encrypted, rotation_window_end)
		 VALUES ($1, $2, $3)`,
		req.Gateway, []byte(req.NewSecret), windowEnd)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"message":        "Secret rotation initiated",
		"window_ends_at": windowEnd.Format(time.RFC3339),
	})
}
