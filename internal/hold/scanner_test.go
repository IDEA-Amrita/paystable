package hold

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/IDEA-Amrita/paystable/internal/gateway"

	_ "github.com/lib/pq"
)

// mockGatewayClient lets each test control exactly what the gateway returns.
type mockGatewayClient struct {
	status string
	amount int64
	err    error
}

func (m *mockGatewayClient) Status(_ context.Context, _ string) (string, int64, json.RawMessage, error) {
	return m.status, m.amount, nil, m.err
}

// setupScannerTestDB creates an in-memory-style test hold row that looks
// expired (expires_at in the past) and returns the txn_id and a cleanup func.
// Requires a real Postgres instance pointed to by TEST_DATABASE_URL.
func setupScannerTestDB(t *testing.T, status string) (*sql.DB, string) {
	t.Helper()
	db := openTestDB(t) // shared helper from main_test.go

	txnID := "scanner_test_" + t.Name() + "_" + time.Now().Format("150405.000000000")

	_, err := db.Exec(`
		INSERT INTO holds (txn_id, gateway, status, amount, currency, read_token,
		                   callback_url, ttl_seconds, expires_at)
		VALUES ($1, 'payu', $2, 5000, 'INR', $3, 'https://merchant.example/cb', 30,
		        now() - interval '1 second')`,
		txnID, status, "pst_rt_"+txnID)
	if err != nil {
		t.Fatalf("insert test hold: %v", err)
	}
	return db, txnID
}

// assertHoldStatus reads the hold status from DB and fails the test if it does not match.
func assertHoldStatus(t *testing.T, db *sql.DB, txnID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT status FROM holds WHERE txn_id=$1`, txnID).Scan(&got); err != nil {
		t.Fatalf("read hold status: %v", err)
	}
	if got != want {
		t.Errorf("hold status = %q, want %q", got, want)
	}
}

// assertOutboxEvent checks that an outbox row for txnID exists with the given event_type.
func assertOutboxEvent(t *testing.T, db *sql.DB, txnID, eventType string) {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT count(*) FROM outbox WHERE txn_id=$1 AND event_type=$2`, txnID, eventType,
	).Scan(&count); err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	if count == 0 {
		t.Errorf("no outbox row for txn_id=%q event_type=%q", txnID, eventType)
	}
}

// assertLedgerSource checks that a ledger row for txnID was written by source='ttl_sweeper'.
func assertLedgerSource(t *testing.T, db *sql.DB, txnID string) {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT count(*) FROM ledger WHERE txn_id=$1 AND source='ttl_sweeper'`, txnID,
	).Scan(&count); err != nil {
		t.Fatalf("query ledger: %v", err)
	}
	if count == 0 {
		t.Errorf("no ledger row with source='ttl_sweeper' for txn_id=%q", txnID)
	}
}

// ── Test Cases ──────────────────────────────────────────────────────────────

// TestCase1: Hold expires, gateway says success + amount matches → CONFIRMED.
func TestScannerConfirmedOnSuccess(t *testing.T) {
	db, txnID := setupScannerTestDB(t, "PENDING")

	factory := func(_ string) gateway.GatewayClient {
		return &mockGatewayClient{status: "success", amount: 5000}
	}
	scanExpiredHolds(context.Background(), db, factory)

	assertHoldStatus(t, db, txnID, "CONFIRMED")
	assertOutboxEvent(t, db, txnID, "transaction.CONFIRMED")
	assertLedgerSource(t, db, txnID)
}

// TestCase2: Hold expires, gateway says failed → FAILED.
func TestScannerFailedOnGatewayFailed(t *testing.T) {
	db, txnID := setupScannerTestDB(t, "PENDING")

	factory := func(_ string) gateway.GatewayClient {
		return &mockGatewayClient{status: "failed", amount: 0}
	}
	scanExpiredHolds(context.Background(), db, factory)

	assertHoldStatus(t, db, txnID, "FAILED")
	assertOutboxEvent(t, db, txnID, "transaction.FAILED")
	assertLedgerSource(t, db, txnID)
}

// TestCase3: Hold expires, gateway call returns error/timeout → INDETERMINATE.
func TestScannerFailedOnGatewayError(t *testing.T) {
	db, txnID := setupScannerTestDB(t, "PENDING")

	factory := func(_ string) gateway.GatewayClient {
		return &mockGatewayClient{err: context.DeadlineExceeded}
	}
	scanExpiredHolds(context.Background(), db, factory)

	assertHoldStatus(t, db, txnID, "INDETERMINATE")
	assertOutboxEvent(t, db, txnID, "transaction.INDETERMINATE")
	assertLedgerSource(t, db, txnID)
}

// TestCase4: Hold expires, gateway says success but amount mismatches → MISMATCH.
func TestScannerIndeterminateOnAmountMismatch(t *testing.T) {
	db, txnID := setupScannerTestDB(t, "PENDING")

	factory := func(_ string) gateway.GatewayClient {
		// Hold amount is 5000; gateway reports 3000 — mismatch.
		return &mockGatewayClient{status: "success", amount: 3000}
	}
	scanExpiredHolds(context.Background(), db, factory)

	assertHoldStatus(t, db, txnID, "MISMATCH")
	assertOutboxEvent(t, db, txnID, "transaction.MISMATCH")
	assertLedgerSource(t, db, txnID)
}

// TestCase5 (bonus): Hold already CONFIRMED before scanner runs → scanner is a no-op.
func TestScannerSkipsAlreadyFinalised(t *testing.T) {
	db, txnID := setupScannerTestDB(t, "CONFIRMED")

	callCount := 0
	factory := func(_ string) gateway.GatewayClient {
		callCount++
		return &mockGatewayClient{status: "success", amount: 5000}
	}
	scanExpiredHolds(context.Background(), db, factory)

	// The status constraint allows CONFIRMED → REFUNDED only, so the scanner
	// must detect CONFIRMED and skip before touching the hold.
	assertHoldStatus(t, db, txnID, "CONFIRMED")
	if callCount > 0 {
		t.Errorf("gateway called %d times, expected 0 — scanner should skip already-finalised holds", callCount)
	}
}
