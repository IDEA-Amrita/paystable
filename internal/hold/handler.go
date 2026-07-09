package hold

import (
	"encoding/json"
	"errors"
	"net/http"
)

type CreateRequest struct {
	TxnID       string          `json:"txn_id"`
	Gateway     string          `json:"gateway"`
	Amount      int64           `json:"amount"`
	Currency    string          `json:"currency"`
	TTLSeconds  int             `json:"ttl_seconds"`
	CallbackURL string          `json:"callback_url"`
	Metadata    json.RawMessage `json:"metadata"`
}

type CreateResponse struct {
	TxnID     string `json:"txn_id"`
	Status    string `json:"status"`
	ReadToken string `json:"read_token"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

type StatusResponse struct {
	TxnID     string `json:"txn_id"`
	Status    string `json:"status"`
	Gateway   string `json:"gateway"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Handler struct {
	store      *Store
	maxTTL     int
	defaultTTL int
	apiKey     string
}

func NewHandler(store *Store, maxTTL int, apiKey string) *Handler {
	return &Handler{store: store, maxTTL: maxTTL, defaultTTL: 300, apiKey: apiKey}
}

// HandleCreate validates the request, applies defaults, and creates the hold.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.TxnID == "" || req.Gateway == "" || req.Amount <= 0 || req.CallbackURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "txn_id, gateway, amount, and callback_url are required"})
		return
	}

	if req.Currency == "" {
		req.Currency = "INR"
	}
	if req.TTLSeconds <= 0 {
		req.TTLSeconds = h.defaultTTL
	}
	if req.TTLSeconds > h.maxTTL {
		req.TTLSeconds = h.maxTTL
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}

	hold, err := h.store.Create(req.TxnID, req.Gateway, req.CallbackURL, req.Currency, req.Amount, req.TTLSeconds, metadata)
	if err != nil {
		if errors.Is(err, ErrCreateConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":   "hold_conflict",
				"message": "txn_id already exists with different create parameters",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create hold"})
		return
	}

	resp := CreateResponse{
		TxnID:     hold.TxnID,
		Status:    hold.Status,
		ReadToken: hold.ReadToken,
		ExpiresAt: hold.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedAt: hold.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	writeJSON(w, http.StatusCreated, resp)
}

// HandleStatus returns the hold after checking the read token or admin key.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	txnID := r.PathValue("txn_id")
	token := r.URL.Query().Get("token")

	if txnID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing txn_id"})
		return
	}
	if token == "" {
		// Allow access via Bearer admin API key
		auth := r.Header.Get("Authorization")
		if h.apiKey != "" && auth == "Bearer "+h.apiKey {
			// Admin access: look up by txn_id only
			hold, err := h.store.GetByTxnID(txnID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "transaction not found"})
				return
			}
			resp := StatusResponse{
				TxnID:     hold.TxnID,
				Status:    hold.Status,
				Gateway:   hold.Gateway,
				Amount:    hold.Amount,
				Currency:  hold.Currency,
				ExpiresAt: hold.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
				CreatedAt: hold.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				UpdatedAt: hold.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing token"})
		return
	}

	hold, err := h.store.GetByTxnIDAndToken(txnID, token)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transaction not found"})
		return
	}

	resp := StatusResponse{
		TxnID:     hold.TxnID,
		Status:    hold.Status,
		Gateway:   hold.Gateway,
		Amount:    hold.Amount,
		Currency:  hold.Currency,
		ExpiresAt: hold.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedAt: hold.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: hold.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
