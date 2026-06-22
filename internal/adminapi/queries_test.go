package adminapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"database/sql"

	"github.com/IDEA-Amrita/paystable/internal/config"
)

func openAdminTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	return db
}

func seedHoldForAdmin(t *testing.T, db *sql.DB, status string, amount int64) string {
	t.Helper()
	txnID := fmt.Sprintf("admin-%d", time.Now().UnixNano())
	_, err := db.Exec(`
		INSERT INTO holds (txn_id, gateway, status, amount, currency, read_token,
		                   callback_url, ttl_seconds, expires_at)
		VALUES ($1, 'payu', $2, $3, 'INR', $4, 'http://cb.test/cb', 300, now()+interval '5m')`,
		txnID, status, amount, "tok_"+txnID)
	if err != nil {
		t.Fatalf("seed hold: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM outbox WHERE txn_id=$1", txnID)             //nolint:errcheck
		db.Exec("DELETE FROM ledger WHERE txn_id=$1", txnID)             //nolint:errcheck
		db.Exec("DELETE FROM verification_polls WHERE txn_id=$1", txnID) //nolint:errcheck
		db.Exec("DELETE FROM holds WHERE txn_id=$1", txnID)              //nolint:errcheck
	})
	return txnID
}

func seedOutboxExhausted(t *testing.T, db *sql.DB, txnID string) int64 {
	t.Helper()
	idem := "evt_" + txnID + "_CONFIRMED"
	var id int64
	err := db.QueryRow(`
		INSERT INTO outbox (txn_id, event_type, payload, idempotency_key, status, attempts, max_attempts)
		VALUES ($1, 'transaction.confirmed', '{"txn_id":"x"}'::jsonb, $2, 'exhausted', 8, 8)
		RETURNING id`, txnID, idem).Scan(&id)
	if err != nil {
		t.Fatalf("seed outbox: %v", err)
	}
	return id
}

func newTestHandler(db *sql.DB) *Handler {
	return &Handler{db: db, cfg: &config.Config{
		Gateway: "payu", StabilizationN: 3, MaxBackoffS: 160,
		HoldMaxTTLS: 900, LogLevel: "info",
		WebhookSecret: "test", GatewayAPIKey: "test", MerchantCallbackSecret: "test",
	}}
}

func loopbackReq(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "127.0.0.1:9999"
	return req
}

func TestOverviewStats_Returns200(t *testing.T) {
	db := openAdminTestDB(t)
	h := newTestHandler(db)

	req := loopbackReq("GET", "/api/v1/admin/overview/stats")
	w := httptest.NewRecorder()
	h.overviewStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"active_holds", "pending_deliveries", "exhausted_deliveries", "rejected_webhooks"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("missing key %q", key)
		}
	}
}

func TestTransactions_EmptySearch(t *testing.T) {
	db := openAdminTestDB(t)
	h := newTestHandler(db)

	req := loopbackReq("GET", "/api/v1/admin/transactions")
	w := httptest.NewRecorder()
	h.transactions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if _, ok := resp["data"]; !ok {
		t.Error("missing data key")
	}
}

func TestTransactions_FilterByStatus(t *testing.T) {
	db := openAdminTestDB(t)
	seedHoldForAdmin(t, db, "CONFIRMED", 49900)
	h := newTestHandler(db)

	req := loopbackReq("GET", "/api/v1/admin/transactions?status=CONFIRMED")
	w := httptest.NewRecorder()
	h.transactions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.Total == 0 {
		t.Error("expected at least one CONFIRMED transaction")
	}
	for _, row := range resp.Data {
		if row["status"] != "CONFIRMED" {
			t.Errorf("got status %q in CONFIRMED filter results", row["status"])
		}
	}
}

func TestTransactionDetail_NotFound(t *testing.T) {
	db := openAdminTestDB(t)
	h := newTestHandler(db)

	req := loopbackReq("GET", "/api/v1/admin/transactions/nonexistent-txn")
	req.SetPathValue("id", "nonexistent-txn")
	w := httptest.NewRecorder()
	h.transactionDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestTransactionDetail_Exists(t *testing.T) {
	db := openAdminTestDB(t)
	txnID := seedHoldForAdmin(t, db, "CONFIRMED", 49900)
	h := newTestHandler(db)

	req := loopbackReq("GET", "/api/v1/admin/transactions/"+txnID)
	req.SetPathValue("id", txnID)
	w := httptest.NewRecorder()
	h.transactionDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["txn_id"] != txnID {
		t.Errorf("txn_id = %v, want %s", resp["txn_id"], txnID)
	}
}

func TestDeliveries_ExhaustedList(t *testing.T) {
	db := openAdminTestDB(t)
	txnID := seedHoldForAdmin(t, db, "CONFIRMED", 49900)
	seedOutboxExhausted(t, db, txnID)
	h := newTestHandler(db)

	req := loopbackReq("GET", "/api/v1/admin/deliveries?status=exhausted")
	w := httptest.NewRecorder()
	h.deliveries(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.Total == 0 {
		t.Error("expected at least one exhausted delivery")
	}
}

func TestReplayDelivery_ResetsToRetry(t *testing.T) {
	db := openAdminTestDB(t)
	txnID := seedHoldForAdmin(t, db, "CONFIRMED", 49900)
	id := seedOutboxExhausted(t, db, txnID)
	h := newTestHandler(db)

	req := loopbackReq("POST", "/api/v1/admin/deliveries/"+strconv.FormatInt(id, 10)+"/replay")
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	h.replayDelivery(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var status string
	db.QueryRow("SELECT status FROM outbox WHERE id=$1", id).Scan(&status) //nolint:errcheck
	if status != "pending" {
		t.Errorf("after replay: status = %q, want pending", status)
	}
}

func TestReplayDelivery_NotFound(t *testing.T) {
	db := openAdminTestDB(t)
	h := newTestHandler(db)

	req := loopbackReq("POST", "/api/v1/admin/deliveries/99999999/replay")
	req.SetPathValue("id", "99999999")
	w := httptest.NewRecorder()
	h.replayDelivery(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestConfig_ReturnsRealValues(t *testing.T) {
	db := openAdminTestDB(t)
	h := newTestHandler(db)

	req := loopbackReq("GET", "/api/v1/admin/config")
	w := httptest.NewRecorder()
	h.config(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	found := false
	for _, item := range resp {
		if item["key"] == "GATEWAY" && item["value"] == "payu" {
			found = true
		}
	}
	if !found {
		t.Error("expected GATEWAY=payu in config response")
	}
}

func TestMismatchStats_Returns200(t *testing.T) {
	db := openAdminTestDB(t)
	h := newTestHandler(db)

	req := loopbackReq("GET", "/api/v1/admin/mismatches/stats")
	w := httptest.NewRecorder()
	h.mismatchStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if _, ok := resp["last_7_days"]; !ok {
		t.Error("missing last_7_days key")
	}
}

func TestLocalhostGate_BlocksNonLoopback(t *testing.T) {
	db := openAdminTestDB(t)
	h := newTestHandler(db)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("GET", "/api/v1/admin/config", nil)
	req.RemoteAddr = "203.0.113.42:1234"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("external IP: got %d, want 403", w.Code)
	}
}

func TestMismatches_Returns200(t *testing.T) {
	db := openAdminTestDB(t)
	h := newTestHandler(db)

	// seed a mismatch: hold CONFIRMED, webhook said failure
	txnID := seedHoldForAdmin(t, db, "CONFIRMED", 49900)
	db.Exec(`INSERT INTO webhooks (txn_id, gateway, event_type, payload) VALUES ($1,'payu','payment.failed','"{}"'::jsonb)`, txnID) //nolint:errcheck

	req := loopbackReq("GET", "/api/v1/admin/mismatches")
	w := httptest.NewRecorder()
	h.mismatches(w, req)

	if w.Code != http.StatusOK {
		body := w.Body.String()
		t.Fatalf("status = %d, want 200. body: %s", w.Code, strings.TrimSpace(body))
	}
}
