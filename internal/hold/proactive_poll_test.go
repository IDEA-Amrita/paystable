package hold

import (
	"database/sql"
	"testing"
	"time"
)

// cleanupPoll removes all verification_polls rows for a txn_id after the test.
func cleanupPoll(t *testing.T, db *sql.DB, txnID string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM verification_polls WHERE txn_id=$1`, txnID); err != nil {
			t.Fatalf("cleanup poll %q: %v", txnID, err)
		}
	})
}

// pollCount returns how many verification_polls rows exist for a txn_id.
func pollCount(t *testing.T, db *sql.DB, txnID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM verification_polls WHERE txn_id=$1`, txnID).Scan(&n); err != nil {
		t.Fatalf("count polls for %q: %v", txnID, err)
	}
	return n
}

// pollRow fetches the single attempt-1 poll row for a txn_id.
func pollRow(t *testing.T, db *sql.DB, txnID string) (status string, scheduledAt time.Time) {
	t.Helper()
	if err := db.QueryRow(
		`SELECT status, scheduled_at FROM verification_polls WHERE txn_id=$1 AND attempt_number=1`,
		txnID,
	).Scan(&status, &scheduledAt); err != nil {
		t.Fatalf("fetch poll row for %q: %v", txnID, err)
	}
	return
}

// TestProactiveCreateOnly verifies that creating a hold queues exactly one
// pending attempt-1 poll without any webhook involvement.
func TestProactiveCreateOnly(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	store := NewStore(db)
	txnID := testTxnID(t)
	cleanupPoll(t, db, txnID)
	cleanupHold(t, db, txnID)

	if _, err := store.Create(txnID, "payu", "https://merchant.example/cb", "INR", 5000, 300, nil); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got := pollCount(t, db, txnID); got != 1 {
		t.Fatalf("poll count = %d, want 1", got)
	}
	status, _ := pollRow(t, db, txnID)
	if status != "pending" {
		t.Fatalf("poll status = %q, want pending", status)
	}
}

// TestProactiveDuplicateCreate verifies that duplicate identical hold create
// requests do not create extra poll rows.
func TestProactiveDuplicateCreate(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	store := NewStore(db)
	txnID := testTxnID(t)
	cleanupPoll(t, db, txnID)
	cleanupHold(t, db, txnID)

	for i := 0; i < 3; i++ {
		if _, err := store.Create(txnID, "payu", "https://merchant.example/cb", "INR", 5000, 300,
			[]byte(`{"order_id":"ord_1"}`)); err != nil {
			t.Fatalf("Create attempt %d: %v", i+1, err)
		}
	}

	if got := pollCount(t, db, txnID); got != 1 {
		t.Fatalf("poll count after 3 duplicate creates = %d, want 1", got)
	}
}

// TestProactiveWebhookNoPoll verifies that a webhook for a txn_id with no
// associated hold does NOT create a poll row (the WHERE EXISTS guard).
func TestProactiveWebhookNoPoll(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	// Use a txn_id that has no hold in the DB.
	txnID := "ghost_" + testTxnID(t)

	// Simulate exactly what handler.go does after a webhook is persisted.
	_, err := db.Exec(`
		INSERT INTO verification_polls (txn_id, attempt_number, scheduled_at, status)
		SELECT $1, 1, now(), 'pending' WHERE EXISTS (SELECT 1 FROM holds WHERE txn_id = $1)
		ON CONFLICT (txn_id, attempt_number) DO UPDATE
			SET scheduled_at = least(verification_polls.scheduled_at, EXCLUDED.scheduled_at)
		WHERE verification_polls.status = 'pending'`, txnID)
	if err != nil {
		t.Fatalf("simulate webhook poll insert: %v", err)
	}

	// No hold → WHERE EXISTS is false → no row should be inserted.
	if got := pollCount(t, db, txnID); got != 0 {
		t.Fatalf("poll count for ghost txn = %d, want 0", got)
	}
}

// TestProactiveCreateThenWebhook verifies that create + webhook together
// produce exactly one attempt-1 poll and that the webhook pulls its
// scheduled_at forward (i.e. the poll fires sooner than the 10-second delay).
func TestProactiveCreateThenWebhook(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	store := NewStore(db)
	txnID := testTxnID(t)
	cleanupPoll(t, db, txnID)
	cleanupHold(t, db, txnID)

	// Step 1: create the hold — this schedules attempt 1 at now()+10s.
	if _, err := store.Create(txnID, "payu", "https://merchant.example/cb", "INR", 5000, 300, nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, proactiveScheduledAt := pollRow(t, db, txnID)

	// Step 2: simulate the webhook arriving (same SQL as handler.go).
	_, err := db.Exec(`
		INSERT INTO verification_polls (txn_id, attempt_number, scheduled_at, status)
		SELECT $1, 1, now(), 'pending' WHERE EXISTS (SELECT 1 FROM holds WHERE txn_id = $1)
		ON CONFLICT (txn_id, attempt_number) DO UPDATE
			SET scheduled_at = least(verification_polls.scheduled_at, EXCLUDED.scheduled_at)
		WHERE verification_polls.status = 'pending'`, txnID)
	if err != nil {
		t.Fatalf("simulate webhook poll update: %v", err)
	}

	// Assert: still exactly 1 row.
	if got := pollCount(t, db, txnID); got != 1 {
		t.Fatalf("poll count after create+webhook = %d, want 1", got)
	}

	// Assert: scheduled_at was pulled forward (earlier than the proactive 10s delay).
	_, updatedScheduledAt := pollRow(t, db, txnID)
	if !updatedScheduledAt.Before(proactiveScheduledAt) {
		t.Fatalf("scheduled_at was not pulled forward: got %s, proactive was %s", updatedScheduledAt, proactiveScheduledAt)
	}
}
