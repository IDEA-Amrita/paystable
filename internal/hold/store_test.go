package hold

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func testTxnID(t *testing.T) string {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return fmt.Sprintf("%s_%d", name, time.Now().UnixNano())
}

func cleanupHold(t *testing.T, db *sql.DB, txnID string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM holds WHERE txn_id=$1`, txnID); err != nil {
			t.Fatalf("cleanup hold %q: %v", txnID, err)
		}
	})
}

func TestStoreCreateDuplicateIdenticalReturnsExisting(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	store := NewStore(db)
	txnID := testTxnID(t)
	cleanupHold(t, db, txnID)

	first, err := store.Create(
		txnID, "payu", "https://merchant.example/cb", "INR", 5000, 300,
		[]byte(`{"order_id":"ord_1","items":[1,2]}`),
	)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	second, err := store.Create(
		txnID, "payu", "https://merchant.example/cb", "INR", 5000, 300,
		[]byte(`{"items":[1,2],"order_id":"ord_1"}`),
	)
	if err != nil {
		t.Fatalf("duplicate create: %v", err)
	}

	if second.ReadToken != first.ReadToken {
		t.Fatalf("read token changed on duplicate create: got %q want %q", second.ReadToken, first.ReadToken)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("created_at changed on duplicate create: got %s want %s", second.CreatedAt, first.CreatedAt)
	}
	if !second.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("expires_at changed on duplicate create: got %s want %s", second.ExpiresAt, first.ExpiresAt)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM holds WHERE txn_id=$1`, txnID).Scan(&count); err != nil {
		t.Fatalf("count holds: %v", err)
	}
	if count != 1 {
		t.Fatalf("holds for txn_id = %d, want 1", count)
	}
}

func TestStoreCreateDuplicateConflict(t *testing.T) {
	tests := []struct {
		name        string
		gateway     string
		callbackURL string
		currency    string
		amount      int64
		ttl         int
		metadata    []byte
	}{
		{
			name:        "amount",
			gateway:     "payu",
			callbackURL: "https://merchant.example/cb",
			currency:    "INR",
			amount:      7000,
			ttl:         300,
			metadata:    []byte(`{"order_id":"ord_1"}`),
		},
		{
			name:        "callback_url",
			gateway:     "payu",
			callbackURL: "https://merchant.example/other",
			currency:    "INR",
			amount:      5000,
			ttl:         300,
			metadata:    []byte(`{"order_id":"ord_1"}`),
		},
		{
			name:        "gateway",
			gateway:     "other",
			callbackURL: "https://merchant.example/cb",
			currency:    "INR",
			amount:      5000,
			ttl:         300,
			metadata:    []byte(`{"order_id":"ord_1"}`),
		},
		{
			name:        "currency",
			gateway:     "payu",
			callbackURL: "https://merchant.example/cb",
			currency:    "USD",
			amount:      5000,
			ttl:         300,
			metadata:    []byte(`{"order_id":"ord_1"}`),
		},
		{
			name:        "ttl",
			gateway:     "payu",
			callbackURL: "https://merchant.example/cb",
			currency:    "INR",
			amount:      5000,
			ttl:         600,
			metadata:    []byte(`{"order_id":"ord_1"}`),
		},
		{
			name:        "metadata",
			gateway:     "payu",
			callbackURL: "https://merchant.example/cb",
			currency:    "INR",
			amount:      5000,
			ttl:         300,
			metadata:    []byte(`{"order_id":"ord_2"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			t.Cleanup(func() { _ = db.Close() })

			store := NewStore(db)
			txnID := testTxnID(t)
			cleanupHold(t, db, txnID)

			_, err := store.Create(
				txnID, "payu", "https://merchant.example/cb", "INR", 5000, 300,
				[]byte(`{"order_id":"ord_1"}`),
			)
			if err != nil {
				t.Fatalf("first create: %v", err)
			}

			_, err = store.Create(txnID, tt.gateway, tt.callbackURL, tt.currency, tt.amount, tt.ttl, tt.metadata)
			if !errors.Is(err, ErrCreateConflict) {
				t.Fatalf("duplicate create error = %v, want ErrCreateConflict", err)
			}

			var amount int64
			var callbackURL string
			if err := db.QueryRow(`SELECT amount, callback_url FROM holds WHERE txn_id=$1`, txnID).Scan(&amount, &callbackURL); err != nil {
				t.Fatalf("read original hold: %v", err)
			}
			if amount != 5000 || callbackURL != "https://merchant.example/cb" {
				t.Fatalf("original hold changed: amount=%d callback_url=%q", amount, callbackURL)
			}
		})
	}
}
