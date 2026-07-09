package hold

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleCreate_MissingFields(t *testing.T) {
	h := NewHandler(nil, 900, "test-api-key")

	tests := []struct {
		name string
		body string
	}{
		{"empty body", `{}`},
		{"missing txn_id", `{"gateway":"payu","amount":100,"callback_url":"http://x.com/cb"}`},
		{"missing gateway", `{"txn_id":"t1","amount":100,"callback_url":"http://x.com/cb"}`},
		{"zero amount", `{"txn_id":"t1","gateway":"payu","amount":0,"callback_url":"http://x.com/cb"}`},
		{"missing callback_url", `{"txn_id":"t1","gateway":"payu","amount":100}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/hold", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.HandleCreate(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want 400", w.Code)
			}
		})
	}
}

func TestHandleCreate_InvalidJSON(t *testing.T) {
	h := NewHandler(nil, 900, "test-api-key")

	req := httptest.NewRequest("POST", "/api/v1/hold", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	h.HandleCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestHandleCreate_DuplicateConflictReturns409(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	txnID := testTxnID(t)
	cleanupHold(t, db, txnID)
	h := NewHandler(NewStore(db), 900, "test-api-key")

	firstBody := fmt.Sprintf(`{
		"txn_id": %q,
		"gateway": "payu",
		"amount": 5000,
		"currency": "INR",
		"ttl_seconds": 300,
		"callback_url": "https://merchant.example/cb",
		"metadata": {"order_id": "ord_1"}
	}`, txnID)
	firstReq := httptest.NewRequest("POST", "/api/v1/hold", strings.NewReader(firstBody))
	firstW := httptest.NewRecorder()
	h.HandleCreate(firstW, firstReq)
	if firstW.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201, body=%s", firstW.Code, firstW.Body.String())
	}

	secondBody := fmt.Sprintf(`{
		"txn_id": %q,
		"gateway": "payu",
		"amount": 7000,
		"currency": "INR",
		"ttl_seconds": 300,
		"callback_url": "https://merchant.example/cb",
		"metadata": {"order_id": "ord_1"}
	}`, txnID)
	secondReq := httptest.NewRequest("POST", "/api/v1/hold", strings.NewReader(secondBody))
	secondW := httptest.NewRecorder()
	h.HandleCreate(secondW, secondReq)
	if secondW.Code != http.StatusConflict {
		t.Fatalf("second create status = %d, want 409, body=%s", secondW.Code, secondW.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(secondW.Body).Decode(&resp); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if resp["error"] != "hold_conflict" {
		t.Fatalf("error = %q, want hold_conflict", resp["error"])
	}
}

func TestHandleCreate_DefaultsApplied(t *testing.T) {
	//validation logic is tested via the missing fields tests above.
	//full create flow requires a database (integration test).
	t.Skip("requires database for full create flow")
}

func TestHandleStatus_MissingToken(t *testing.T) {
	h := NewHandler(nil, 900, "test-api-key")

	req := httptest.NewRequest("GET", "/api/v1/transactions/txn_123/status", nil)
	req.SetPathValue("txn_id", "txn_123")
	w := httptest.NewRecorder()
	h.HandleStatus(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", w.Code)
	}
}

func TestHandleStatus_NotFound(t *testing.T) {
	//nil store will panic on method call, so we need a real store.
	//skip DB dependent tests here; will test them while writing integration tests :)
	t.Skip("requires database")
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
}
