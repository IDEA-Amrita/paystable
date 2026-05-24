package webhook

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"

	"github.com/IDEA-Amrita/paystable/internal/gateway/payu"
)

type Handler struct {
	db     *sql.DB
	secret string
}

func NewHandler(db *sql.DB, secret string) *Handler {
	return &Handler{db: db, secret: secret}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	gateway := r.PathValue("gateway")
	if gateway == "" {
		http.Error(w, "missing gateway", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	params, err := parsePayload(body, r.Header.Get("Content-Type"))
	if err != nil {
		h.quarantine(gateway, "malformed_payload", r, body)
		w.WriteHeader(http.StatusOK)
		return
	}

	if !h.verify(gateway, params) {
		h.quarantine(gateway, "hmac_mismatch", r, body)
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.persist(gateway, params); err != nil {
		slog.Error("failed to persist webhook", "error", err, "gateway", gateway)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) verify(gateway string, params map[string]string) bool {
	switch gateway {
	case "payu":
		return payu.VerifyResponseHash(params, h.secret)
	default:
		return false
	}
}

func (h *Handler) persist(gateway string, params map[string]string) error {
	txnID := extractTxnID(gateway, params)
	eventType := extractEventType(gateway, params)
	gatewayEventID := params["mihpayid"]

	payload, _ := json.Marshal(params)

	_, err := h.db.Exec(`
		INSERT INTO webhooks (txn_id, gateway, gateway_event_id, event_type, payload)
		SELECT $1, $2, $3, $4, $5::jsonb
		WHERE EXISTS (SELECT 1 FROM holds WHERE txn_id = $1)`,
		txnID, gateway, gatewayEventID, eventType, payload)

	if err != nil {
		return err
	}

	slog.Info("webhook persisted", "gateway", gateway, "txn_id", txnID, "event", eventType)
	return nil
}

func (h *Handler) quarantine(gateway, reason string, r *http.Request, body []byte) {
	headers, _ := json.Marshal(r.Header)
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)

	_, err := h.db.Exec(`
		INSERT INTO webhooks_rejected (gateway, rejection_reason, headers, raw_body, source_ip)
		VALUES ($1, $2, $3::jsonb, $4, $5::inet)`,
		gateway, reason, headers, body, ip)

	if err != nil {
		slog.Error("failed to quarantine webhook", "error", err, "gateway", gateway)
	} else {
		slog.Warn("webhook quarantined", "gateway", gateway, "reason", reason)
	}
}

func extractTxnID(gateway string, params map[string]string) string {
	switch gateway {
	case "payu":
		return params["txnid"]
	default:
		return params["txnid"]
	}
}

func extractEventType(gateway string, params map[string]string) string {
	switch gateway {
	case "payu":
		return "payment." + params["status"]
	default:
		return "unknown"
	}
}

func parsePayload(body []byte, contentType string) (map[string]string, error) {
	params := make(map[string]string)

	if contentType == "application/json" || (len(body) > 0 && body[0] == '{') {
		if err := json.Unmarshal(body, &params); err != nil {
			return nil, err
		}
		return params, nil
	}

	//payu sends application/x-www-form-urlencoded
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	for k, v := range values {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	return params, nil
}
