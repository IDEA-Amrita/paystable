package stabilizer

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/IDEA-Amrita/paystable/internal/gateway"
)

type fakeClient struct {
	status string
	amount int64
	err    error
}

func (f fakeClient) Status(_ context.Context, _ string) (string, int64, json.RawMessage, error) {
	return f.status, f.amount, nil, f.err
}

func factory(c gateway.GatewayClient) func(string) gateway.GatewayClient {
	return func(string) gateway.GatewayClient { return c }
}

func ttlTestDB(t *testing.T) *sql.DB {
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

func seedExpiredHold(t *testing.T, db *sql.DB, amount int64) string {
	t.Helper()
	return seedTTLHold(t, db, "VERIFYING", amount, true)
}

func seedTTLHold(t *testing.T, db *sql.DB, status string, amount int64, expired bool) string {
	t.Helper()
	txnID := "ttl-" + strconv.Itoa(int(time.Now().UnixNano()))
	expiresAt := time.Now().Add(time.Minute)
	if expired {
		expiresAt = time.Now().Add(-time.Minute)
	}
	_, err := db.Exec(`
		INSERT INTO holds (txn_id, gateway, status, amount, currency, read_token,
		                   callback_url, ttl_seconds, expires_at, metadata)
		VALUES ($1, 'payu', $2, $3, 'INR', $4, 'http://x/cb', 300, $5, $6::jsonb)`,
		txnID, status, amount, "tok_"+txnID, expiresAt, []byte(`{"source":"ttl_test"}`))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM outbox WHERE txn_id=$1", txnID)             //nolint:errcheck
		db.Exec("DELETE FROM ledger WHERE txn_id=$1", txnID)             //nolint:errcheck
		db.Exec("DELETE FROM verification_polls WHERE txn_id=$1", txnID) //nolint:errcheck
		db.Exec("DELETE FROM holds WHERE txn_id=$1", txnID)              //nolint:errcheck
	})
	return txnID
}

func holdStatusOf(t *testing.T, db *sql.DB, txnID string) string {
	t.Helper()
	var s string
	db.QueryRow("SELECT status FROM holds WHERE txn_id=$1", txnID).Scan(&s) //nolint:errcheck
	return s
}

func requireLedgerTransition(t *testing.T, db *sql.DB, txnID, wantSource, wantToStatus string) {
	t.Helper()
	var source, toStatus string
	if err := db.QueryRow(`
		SELECT source, to_status
		FROM ledger
		WHERE txn_id=$1 AND event_type='state_transition'
		ORDER BY created_at DESC
		LIMIT 1`, txnID).Scan(&source, &toStatus); err != nil {
		t.Fatalf("read ledger transition: %v", err)
	}
	if source != wantSource || toStatus != wantToStatus {
		t.Fatalf("ledger transition = source:%q to:%q, want source:%q to:%q", source, toStatus, wantSource, wantToStatus)
	}
}

func requireOutboxPayload(t *testing.T, db *sql.DB, txnID, wantEventType, wantPayloadEvent, wantStatus string) {
	t.Helper()
	var eventType string
	var payload json.RawMessage
	if err := db.QueryRow(`
		SELECT event_type, payload
		FROM outbox
		WHERE txn_id=$1
		ORDER BY created_at DESC
		LIMIT 1`, txnID).Scan(&eventType, &payload); err != nil {
		t.Fatalf("read outbox payload: %v", err)
	}
	if eventType != wantEventType {
		t.Fatalf("outbox event_type = %q, want %q", eventType, wantEventType)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("decode outbox payload: %v", err)
	}
	if body["event"] != wantPayloadEvent {
		t.Fatalf("payload event = %v, want %s", body["event"], wantPayloadEvent)
	}
	if body["status"] != wantStatus {
		t.Fatalf("payload status = %v, want %s", body["status"], wantStatus)
	}
	if body["gateway"] != "payu" {
		t.Fatalf("payload gateway = %v, want payu", body["gateway"])
	}
	if body["currency"] != "INR" {
		t.Fatalf("payload currency = %v, want INR", body["currency"])
	}
}

func TestResolveExpiredHold_SuccessMatch_Confirmed(t *testing.T) {
	db := ttlTestDB(t)
	txnID := seedExpiredHold(t, db, 49900)

	resolveExpiredHold(context.Background(), db,
		expiredHold{TxnID: txnID, Gateway: "payu", Amount: 49900},
		factory(fakeClient{status: "success", amount: 49900}))

	if got := holdStatusOf(t, db, txnID); got != "CONFIRMED" {
		t.Errorf("status = %q, want CONFIRMED", got)
	}
	requireLedgerTransition(t, db, txnID, "stabilizer", "CONFIRMED")
	requireOutboxPayload(t, db, txnID, "transaction.CONFIRMED", "transaction.confirmed", "CONFIRMED")
}

func TestResolveExpiredHold_SuccessAmountMismatch_Mismatch(t *testing.T) {
	db := ttlTestDB(t)
	txnID := seedExpiredHold(t, db, 49900)

	resolveExpiredHold(context.Background(), db,
		expiredHold{TxnID: txnID, Gateway: "payu", Amount: 49900},
		factory(fakeClient{status: "success", amount: 25000}))

	if got := holdStatusOf(t, db, txnID); got != "MISMATCH" {
		t.Errorf("status = %q, want MISMATCH", got)
	}
	requireLedgerTransition(t, db, txnID, "stabilizer", "MISMATCH")
	requireOutboxPayload(t, db, txnID, "transaction.MISMATCH", "transaction.mismatch", "MISMATCH")
}

func TestResolveExpiredHold_Failed(t *testing.T) {
	db := ttlTestDB(t)
	txnID := seedExpiredHold(t, db, 49900)

	resolveExpiredHold(context.Background(), db,
		expiredHold{TxnID: txnID, Gateway: "payu", Amount: 49900},
		factory(fakeClient{status: "failed"}))

	if got := holdStatusOf(t, db, txnID); got != "FAILED" {
		t.Errorf("status = %q, want FAILED", got)
	}
	requireLedgerTransition(t, db, txnID, "stabilizer", "FAILED")
	requireOutboxPayload(t, db, txnID, "transaction.FAILED", "transaction.failed", "FAILED")
}

func TestResolveExpiredHold_Pending_Indeterminate(t *testing.T) {
	db := ttlTestDB(t)
	txnID := seedExpiredHold(t, db, 49900)

	resolveExpiredHold(context.Background(), db,
		expiredHold{TxnID: txnID, Gateway: "payu", Amount: 49900},
		factory(fakeClient{status: "pending"}))

	if got := holdStatusOf(t, db, txnID); got != "INDETERMINATE" {
		t.Errorf("status = %q, want INDETERMINATE (pending is not a confirmed failure)", got)
	}
	requireLedgerTransition(t, db, txnID, "stabilizer", "INDETERMINATE")
	requireOutboxPayload(t, db, txnID, "transaction.INDETERMINATE", "transaction.indeterminate", "INDETERMINATE")
}

func TestClaimExpiredHolds_SkipsTerminalHolds(t *testing.T) {
	db := ttlTestDB(t)
	txnID := seedTTLHold(t, db, "CONFIRMED", 49900, true)

	holds, err := claimExpiredHolds(context.Background(), db)
	if err != nil {
		t.Fatalf("claim expired holds: %v", err)
	}
	for _, h := range holds {
		if h.TxnID == txnID {
			t.Fatalf("terminal hold %q was claimed", txnID)
		}
	}
}
