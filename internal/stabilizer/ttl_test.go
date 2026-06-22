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
	txnID := "ttl-" + strconv.Itoa(int(time.Now().UnixNano()))
	_, err := db.Exec(`
		INSERT INTO holds (txn_id, gateway, status, amount, currency, read_token,
		                   callback_url, ttl_seconds, expires_at)
		VALUES ($1, 'payu', 'VERIFYING', $2, 'INR', $3, 'http://x/cb', 300, now()-interval '1 minute')`,
		txnID, amount, "tok_"+txnID)
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

func TestResolveExpiredHold_SuccessMatch_Confirmed(t *testing.T) {
	db := ttlTestDB(t)
	txnID := seedExpiredHold(t, db, 49900)

	resolveExpiredHold(context.Background(), db,
		expiredHold{TxnID: txnID, Gateway: "payu", Amount: 49900},
		factory(fakeClient{status: "success", amount: 49900}))

	if got := holdStatusOf(t, db, txnID); got != "CONFIRMED" {
		t.Errorf("status = %q, want CONFIRMED", got)
	}
}

func TestResolveExpiredHold_SuccessAmountMismatch_Indeterminate(t *testing.T) {
	db := ttlTestDB(t)
	txnID := seedExpiredHold(t, db, 49900)

	resolveExpiredHold(context.Background(), db,
		expiredHold{TxnID: txnID, Gateway: "payu", Amount: 49900},
		factory(fakeClient{status: "success", amount: 25000}))

	if got := holdStatusOf(t, db, txnID); got != "INDETERMINATE" {
		t.Errorf("status = %q, want INDETERMINATE", got)
	}
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
}
