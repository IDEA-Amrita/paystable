package stabilizer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/IDEA-Amrita/paystable/internal/config"
	"github.com/IDEA-Amrita/paystable/internal/gateway"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// openWorkerTestDB opens a DB and skips the test if DATABASE_URL is absent.
func openWorkerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set – skipping worker integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("db.Ping: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// defaultCfg returns a minimal config suitable for unit testing processPoll.
// StabilizationN: 3 matches the production default — 3 consecutive non-success
// observations are required before a hold is marked FAILED.
func defaultCfg() *config.Config {
	return &config.Config{StabilizationN: 3}
}

// seedHold inserts a hold row and returns its txn_id.
// Cleans up after the test automatically.
func seedHold(t *testing.T, db *sql.DB, txnID, gw string, amount int64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO holds (txn_id, gateway, status, amount, currency, read_token,
		                   callback_url, ttl_seconds, expires_at)
		VALUES ($1, $2, 'VERIFYING', $3, 'INR', $4, 'http://localhost/cb', 300, now()+interval '5 minutes')
	`, txnID, gw, amount, "tok_"+txnID)
	if err != nil {
		t.Fatalf("seedHold(%s): %v", txnID, err)
	}
	t.Cleanup(func() {
		// Delete in dependency order (child tables first).
		db.Exec("DELETE FROM outbox WHERE txn_id=$1", txnID)             //nolint:errcheck
		db.Exec("DELETE FROM ledger WHERE txn_id=$1", txnID)             //nolint:errcheck
		db.Exec("DELETE FROM verification_polls WHERE txn_id=$1", txnID) //nolint:errcheck
		db.Exec("DELETE FROM holds WHERE txn_id=$1", txnID)              //nolint:errcheck
	})
}

// seedPoll inserts a verification_polls row and returns its id.
func seedPoll(t *testing.T, db *sql.DB, txnID string, attempt int) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO verification_polls (txn_id, attempt_number, scheduled_at, status)
		VALUES ($1, $2, now(), 'pending')
		RETURNING id
	`, txnID, attempt).Scan(&id)
	if err != nil {
		t.Fatalf("seedPoll(%s, attempt=%d): %v", txnID, attempt, err)
	}
	return id
}

func seedCompletedPoll(t *testing.T, db *sql.DB, txnID string, attempt int, status string, amount int64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO verification_polls
		    (txn_id, attempt_number, scheduled_at, status, gateway_status, gateway_amount, completed_at)
		VALUES ($1, $2, now(), 'completed', $3, $4, now() - ($5 * interval '1 second'))
	`, txnID, attempt, status, amount, 10-attempt)
	if err != nil {
		t.Fatalf("seedCompletedPoll(%s, attempt=%d): %v", txnID, attempt, err)
	}
}

// holdStatus fetches the current status of a hold.
func holdStatus(t *testing.T, db *sql.DB, txnID string) string {
	t.Helper()
	var s string
	if err := db.QueryRow("SELECT status FROM holds WHERE txn_id=$1", txnID).Scan(&s); err != nil {
		t.Fatalf("holdStatus(%s): %v", txnID, err)
	}
	return s
}

// pollStatus fetches the status of a specific poll row.
func pollStatus(t *testing.T, db *sql.DB, pollID int64) string {
	t.Helper()
	var s string
	if err := db.QueryRow("SELECT status FROM verification_polls WHERE id=$1", pollID).Scan(&s); err != nil {
		t.Fatalf("pollStatus(%d): %v", pollID, err)
	}
	return s
}

// countPendingPolls returns number of pending polls for a txn.
func countPendingPolls(t *testing.T, db *sql.DB, txnID string) int {
	t.Helper()
	var n int
	db.QueryRow("SELECT COUNT(*) FROM verification_polls WHERE txn_id=$1 AND status='pending'", txnID).Scan(&n) //nolint:errcheck
	return n
}

// outboxEventType fetches the most recent outbox event_type for a txn.
func outboxEventType(t *testing.T, db *sql.DB, txnID string) string {
	t.Helper()
	var et sql.NullString
	db.QueryRow("SELECT event_type FROM outbox WHERE txn_id=$1 ORDER BY created_at DESC LIMIT 1", txnID).Scan(&et) //nolint:errcheck
	return et.String
}

// ledgerEntries returns all ledger to_status values for a txn in order.
func ledgerEntries(t *testing.T, db *sql.DB, txnID string) []string {
	t.Helper()
	rows, _ := db.Query("SELECT to_status FROM ledger WHERE txn_id=$1 ORDER BY created_at", txnID)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		rows.Scan(&s) //nolint:errcheck
		out = append(out, s)
	}
	return out
}

// ── Mock gateway client ───────────────────────────────────────────────────────

type mockClient struct {
	status string
	amount int64
	err    error
	calls  int
}

func (m *mockClient) Status(_ context.Context, _ string) (string, int64, json.RawMessage, error) {
	m.calls++
	return m.status, m.amount, json.RawMessage(`{}`), m.err
}

func mockFactory(gw string, mc *mockClient) func(string) gateway.GatewayClient {
	return func(name string) gateway.GatewayClient {
		if name == gw {
			return mc
		}
		return nil
	}
}

// pollRow builds the anonymous struct that processPoll expects.
func pollRow(id int64, txnID string, attempt int, gw string, holdAmount int64) struct {
	ID         int64
	TxnID      string
	Attempt    int
	Gateway    string
	HoldAmount int64
	CreatedAt  time.Time
} {
	return struct {
		ID         int64
		TxnID      string
		Attempt    int
		Gateway    string
		HoldAmount int64
		CreatedAt  time.Time
	}{
		ID:         id,
		TxnID:      txnID,
		Attempt:    attempt,
		Gateway:    gw,
		HoldAmount: holdAmount,
		CreatedAt:  time.Now().Add(-5 * time.Second), // simulate webhook arrived 5s ago
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// 1. Success + amount match after a stable streak → hold is CONFIRMED.
func TestProcessPoll_SuccessAmountMatch_Confirmed(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_success_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 10000)
	seedCompletedPoll(t, db, txnID, 1, "success", 10000)
	seedCompletedPoll(t, db, txnID, 2, "success", 10000)
	pollID := seedPoll(t, db, txnID, 3)

	// Seed rate_limits so AcquireToken succeeds.
	db.Exec("DELETE FROM rate_limits WHERE gateway='payu'")                                                                               //nolint:errcheck
	db.Exec("INSERT INTO rate_limits (gateway,tokens,last_refill) VALUES ('payu',10,now()) ON CONFLICT(gateway) DO UPDATE SET tokens=10") //nolint:errcheck

	mc := &mockClient{status: "success", amount: 10000}
	lag := NewLagEstimator()
	cfg := defaultCfg()

	p := pollRow(pollID, txnID, 3, "payu", 10000)
	processPoll(context.Background(), db, cfg, lag, p, mockFactory("payu", mc))
	time.Sleep(100 * time.Millisecond) // let goroutine complete

	if got := holdStatus(t, db, txnID); got != "CONFIRMED" {
		t.Errorf("hold status = %q, want CONFIRMED", got)
	}
	if ps := pollStatus(t, db, pollID); ps != "completed" {
		t.Errorf("poll status = %q, want completed", ps)
	}
	if et := outboxEventType(t, db, txnID); et != "transaction.CONFIRMED" {
		t.Errorf("outbox event = %q, want transaction.CONFIRMED", et)
	}
	if entries := ledgerEntries(t, db, txnID); len(entries) == 0 || entries[len(entries)-1] != "CONFIRMED" {
		t.Errorf("ledger entries = %v, want last = CONFIRMED", entries)
	}
	// Lag sample must have been recorded.
	if lag.SampleCount("payu") != 1 {
		t.Errorf("lag samples = %d, want 1", lag.SampleCount("payu"))
	}
}

// 2. Success but AMOUNT MISMATCH → hold is MISMATCH, NOT CONFIRMED.
func TestProcessPoll_SuccessAmountMismatch_Indeterminate(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_mismatch_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 10000)
	pollID := seedPoll(t, db, txnID, 1)

	db.Exec("DELETE FROM rate_limits WHERE gateway='payu'")                                                                               //nolint:errcheck
	db.Exec("INSERT INTO rate_limits (gateway,tokens,last_refill) VALUES ('payu',10,now()) ON CONFLICT(gateway) DO UPDATE SET tokens=10") //nolint:errcheck

	mc := &mockClient{status: "success", amount: 500} // 500 ≠ 10000
	lag := NewLagEstimator()

	p := pollRow(pollID, txnID, 1, "payu", 10000)
	processPoll(context.Background(), db, defaultCfg(), lag, p, mockFactory("payu", mc))
	time.Sleep(100 * time.Millisecond)

	if got := holdStatus(t, db, txnID); got != "MISMATCH" {
		t.Errorf("hold status = %q, want MISMATCH", got)
	}
	// No lag sample — we did not confirm.
	if lag.SampleCount("payu") != 0 {
		t.Errorf("lag samples = %d, want 0 on mismatch", lag.SampleCount("payu"))
	}
	if et := outboxEventType(t, db, txnID); et != "transaction.MISMATCH" {
		t.Errorf("outbox event = %q, want transaction.MISMATCH", et)
	}
}

//  3. Gateway returns "failed" on attempt 1 → poll is marked failed,
//     a NEW pending poll is inserted for attempt 2 (replica-lag patience).
func TestProcessPoll_GatewayFailed_SchedulesRetry(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_retry_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 10000)
	pollID := seedPoll(t, db, txnID, 1)

	db.Exec("DELETE FROM rate_limits WHERE gateway='payu'")                                                                               //nolint:errcheck
	db.Exec("INSERT INTO rate_limits (gateway,tokens,last_refill) VALUES ('payu',10,now()) ON CONFLICT(gateway) DO UPDATE SET tokens=10") //nolint:errcheck

	// Gateway says failed — this might be stale replica data.
	mc := &mockClient{status: "failed", amount: 0}
	lag := NewLagEstimator()

	p := pollRow(pollID, txnID, 1, "payu", 10000)
	processPoll(context.Background(), db, defaultCfg(), lag, p, mockFactory("payu", mc))
	time.Sleep(100 * time.Millisecond)

	// Hold must still be VERIFYING — do NOT declare failure on a single replica observation.
	if got := holdStatus(t, db, txnID); got != "VERIFYING" {
		t.Errorf("hold status = %q, want VERIFYING (must not fail on single 'failed' from replica)", got)
	}
	// A new pending poll must have been scheduled.
	if n := countPendingPolls(t, db, txnID); n < 1 {
		t.Errorf("pending polls after retry-schedule = %d, want >= 1", n)
	}
	if ps := pollStatus(t, db, pollID); ps != "completed" {
		t.Errorf("original poll status = %q, want completed", ps)
	}
}

//  4. Gateway errors (network failure) on attempt 1 → poll marked failed,
//     retry poll inserted. Hold stays VERIFYING.
func TestProcessPoll_GatewayError_SchedulesRetry(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_gwerr_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 5000)
	pollID := seedPoll(t, db, txnID, 1)

	db.Exec("DELETE FROM rate_limits WHERE gateway='payu'")                                                                               //nolint:errcheck
	db.Exec("INSERT INTO rate_limits (gateway,tokens,last_refill) VALUES ('payu',10,now()) ON CONFLICT(gateway) DO UPDATE SET tokens=10") //nolint:errcheck

	mc := &mockClient{err: fmt.Errorf("connection refused")}
	lag := NewLagEstimator()

	p := pollRow(pollID, txnID, 1, "payu", 5000)
	processPoll(context.Background(), db, defaultCfg(), lag, p, mockFactory("payu", mc))
	time.Sleep(100 * time.Millisecond)

	if got := holdStatus(t, db, txnID); got != "VERIFYING" {
		t.Errorf("hold status = %q, want VERIFYING after network error", got)
	}
	if ps := pollStatus(t, db, pollID); ps != "failed" {
		t.Errorf("poll status = %q, want failed after gateway error", ps)
	}
	if n := countPendingPolls(t, db, txnID); n < 1 {
		t.Errorf("pending polls = %d, want >= 1 (retry must be scheduled)", n)
	}
}

// 5. Final gateway error without a verifiable answer → INDETERMINATE.
func TestProcessPoll_FinalAttemptGatewayError_Exhausted(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_exhaust_gw_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 5000)
	pollID := seedPoll(t, db, txnID, 3)

	db.Exec("DELETE FROM rate_limits WHERE gateway='payu'")                                                                               //nolint:errcheck
	db.Exec("INSERT INTO rate_limits (gateway,tokens,last_refill) VALUES ('payu',10,now()) ON CONFLICT(gateway) DO UPDATE SET tokens=10") //nolint:errcheck

	mc := &mockClient{err: fmt.Errorf("gateway timeout")}
	lag := NewLagEstimator()

	p := pollRow(pollID, txnID, 3, "payu", 5000)
	processPoll(context.Background(), db, defaultCfg(), lag, p, mockFactory("payu", mc))
	time.Sleep(100 * time.Millisecond)

	if got := holdStatus(t, db, txnID); got != "INDETERMINATE" {
		t.Errorf("hold status = %q, want INDETERMINATE after final gateway error", got)
	}
	if et := outboxEventType(t, db, txnID); et != "transaction.INDETERMINATE" {
		t.Errorf("outbox event = %q, want transaction.INDETERMINATE", et)
	}
	// No new pending poll must be added.
	if n := countPendingPolls(t, db, txnID); n != 0 {
		t.Errorf("pending polls = %d, want 0 after exhaustion", n)
	}
}

// 6. A stable failed streak becomes FAILED.
func TestProcessPoll_FinalAttemptNoConsensus_Exhausted(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_exhaust_nc_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 7500)
	seedCompletedPoll(t, db, txnID, 1, "failed", 0)
	seedCompletedPoll(t, db, txnID, 2, "failed", 0)
	pollID := seedPoll(t, db, txnID, 3)

	db.Exec("DELETE FROM rate_limits WHERE gateway='payu'")                                                                               //nolint:errcheck
	db.Exec("INSERT INTO rate_limits (gateway,tokens,last_refill) VALUES ('payu',10,now()) ON CONFLICT(gateway) DO UPDATE SET tokens=10") //nolint:errcheck

	mc := &mockClient{status: "failed", amount: 0}
	lag := NewLagEstimator()

	p := pollRow(pollID, txnID, 3, "payu", 7500)
	processPoll(context.Background(), db, defaultCfg(), lag, p, mockFactory("payu", mc))
	time.Sleep(100 * time.Millisecond)

	if got := holdStatus(t, db, txnID); got != "FAILED" {
		t.Errorf("hold status = %q, want FAILED after stable failure", got)
	}
	if n := countPendingPolls(t, db, txnID); n != 0 {
		t.Errorf("pending polls = %d, want 0 — no more retries after attempt 5", n)
	}
}

//  7. Nil gateway client (unknown gateway name) → poll marked failed,
//     hold stays VERIFYING, no crash.
func TestProcessPoll_NilClient_DoesNotPanic(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_nilclient_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "unknown_gw", 1000)
	pollID := seedPoll(t, db, txnID, 1)

	db.Exec("DELETE FROM rate_limits WHERE gateway='unknown_gw'")                                                                               //nolint:errcheck
	db.Exec("INSERT INTO rate_limits (gateway,tokens,last_refill) VALUES ('unknown_gw',10,now()) ON CONFLICT(gateway) DO UPDATE SET tokens=10") //nolint:errcheck

	lag := NewLagEstimator()
	// Factory returns nil for any gateway.
	factory := func(string) gateway.GatewayClient { return nil }

	p := pollRow(pollID, txnID, 1, "unknown_gw", 1000)
	processPoll(context.Background(), db, defaultCfg(), lag, p, factory)
	time.Sleep(100 * time.Millisecond)

	if ps := pollStatus(t, db, pollID); ps != "failed" {
		t.Errorf("poll status = %q, want failed when client is nil", ps)
	}
	if got := holdStatus(t, db, txnID); got != "VERIFYING" {
		t.Errorf("hold status = %q, want VERIFYING (nil client is not a terminal failure)", got)
	}
}

//  8. finalizeHold idempotency: calling it twice for the same txn must not
//     create duplicate ledger or outbox rows.
func TestFinalizeHold_Idempotent(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_idempotent_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 2000)

	if err := finalizeHold(context.Background(), db, txnID); err != nil {
		t.Fatalf("first finalizeHold: %v", err)
	}
	if err := finalizeHold(context.Background(), db, txnID); err != nil {
		t.Fatalf("second finalizeHold: %v", err)
	}

	// Exactly one ledger entry must exist.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM ledger WHERE txn_id=$1 AND to_status='CONFIRMED'", txnID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("ledger CONFIRMED entries = %d, want 1 (idempotent)", count)
	}
	// Exactly one outbox event.
	db.QueryRow("SELECT COUNT(*) FROM outbox WHERE txn_id=$1 AND event_type='transaction.CONFIRMED'", txnID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("outbox CONFIRMED events = %d, want 1 (idempotent)", count)
	}
}

// 9. markHoldExhausted idempotency: repeated calls must not duplicate rows.
func TestMarkHoldExhausted_Idempotent(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_exhaust_idem_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 3000)

	reason := "polling_exhausted_no_consensus"
	for i := 0; i < 3; i++ {
		if err := markHoldExhausted(context.Background(), db, txnID, reason); err != nil {
			t.Fatalf("call %d: markHoldExhausted: %v", i+1, err)
		}
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM ledger WHERE txn_id=$1 AND to_status='FAILED'", txnID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("ledger FAILED entries = %d, want 1", count)
	}
	db.QueryRow("SELECT COUNT(*) FROM outbox WHERE txn_id=$1 AND event_type='transaction.FAILED'", txnID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("outbox FAILED events = %d, want 1", count)
	}
}

// 10. markHoldMismatch: amount mismatch writes correct detail to ledger.
func TestMarkHoldIndeterminate_LedgerDetail(t *testing.T) {
	db := openWorkerTestDB(t)
	txnID := fmt.Sprintf("wp_indet_%d", time.Now().UnixNano())
	seedHold(t, db, txnID, "payu", 10000)

	if err := markHoldMismatch(context.Background(), db, txnID, 500, 10000); err != nil {
		t.Fatalf("markHoldMismatch: %v", err)
	}

	if got := holdStatus(t, db, txnID); got != "MISMATCH" {
		t.Errorf("hold status = %q, want MISMATCH", got)
	}

	var rawDetail string
	db.QueryRow("SELECT detail::text FROM ledger WHERE txn_id=$1 AND to_status='MISMATCH'", txnID).Scan(&rawDetail) //nolint:errcheck

	var detail map[string]interface{}
	if err := json.Unmarshal([]byte(rawDetail), &detail); err != nil {
		t.Fatalf("detail is not valid JSON: %v", err)
	}
	if detail["reason"] != "amount_mismatch" {
		t.Errorf("detail.reason = %v, want amount_mismatch", detail["reason"])
	}
	if int64(detail["gateway_amount"].(float64)) != 500 {
		t.Errorf("detail.gateway_amount = %v, want 500", detail["gateway_amount"])
	}
	if int64(detail["hold_amount"].(float64)) != 10000 {
		t.Errorf("detail.hold_amount = %v, want 10000", detail["hold_amount"])
	}
}

// 11. isSuccessStatus: covers all known success strings and rejects others.
func TestIsSuccessStatus(t *testing.T) {
	for _, s := range []string{"success", "captured", "completed", "SUCCESS"} {
		if !isSuccessStatus(s) {
			t.Errorf("isSuccessStatus(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"failed", "failure", "pending", "not_found", ""} {
		if isSuccessStatus(s) {
			t.Errorf("isSuccessStatus(%q) = true, want false", s)
		}
	}
}
