package stabilizer

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// openTestDB opens a postgres connection from DATABASE_URL env var.
// If the env var is absent the calling test is skipped.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set – skipping DB integration test")
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

// cleanRateLimit removes any leftover row for the given gateway so each
// test case starts from a known-clean state.
func cleanRateLimit(t *testing.T, db *sql.DB, gateway string) {
	t.Helper()
	if _, err := db.Exec("DELETE FROM rate_limits WHERE gateway=$1", gateway); err != nil {
		t.Fatalf("cleanRateLimit: %v", err)
	}
}

// ── 1. First acquisition creates the row and succeeds ─────────────────────────

func TestAcquireToken_FirstCallCreatesRow(t *testing.T) {
	db := openTestDB(t)
	gw := "tb_test_first_call"
	cleanRateLimit(t, db, gw)

	ctx := context.Background()
	acquired, _, err := AcquireToken(ctx, db, gw, 5, 1.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !acquired {
		t.Fatal("expected token to be acquired on first call")
	}

	// Row must exist with capacity-1 tokens remaining.
	var tokens float64
	if err := db.QueryRow("SELECT tokens FROM rate_limits WHERE gateway=$1", gw).Scan(&tokens); err != nil {
		t.Fatalf("querying tokens: %v", err)
	}
	if tokens != 4.0 {
		t.Errorf("tokens after first acquire = %.1f, want 4.0", tokens)
	}
}

// ── 2. Token drain: drain all tokens then verify next call is rate-limited ────

func TestAcquireToken_DrainThenRateLimit(t *testing.T) {
	db := openTestDB(t)
	gw := "tb_test_drain"
	cleanRateLimit(t, db, gw)

	ctx := context.Background()
	capacity := 3

	// Drain all tokens.
	for i := 0; i < capacity; i++ {
		ok, _, err := AcquireToken(ctx, db, gw, capacity, 1.0)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
		if !ok {
			t.Fatalf("call %d: expected acquired=true while tokens remain", i+1)
		}
	}

	// Next call must be rate-limited.
	ok, nextAvail, err := AcquireToken(ctx, db, gw, capacity, 1.0)
	if err != nil {
		t.Fatalf("rate-limited call: unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected acquired=false after bucket drained")
	}
	if nextAvail.IsZero() {
		t.Fatal("expected non-zero nextAvailable when rate-limited")
	}
}

// ── 3. nextAvailable is anchored from NOW, not lastRefill ─────────────────────
//
// Before the bug fix, nextAvailable = lastRefill + wait, which put the wait
// time in the past. After the fix it must be strictly in the future.

func TestAcquireToken_WaitTimeIsInFuture(t *testing.T) {
	db := openTestDB(t)
	gw := "tb_test_wait_time"
	cleanRateLimit(t, db, gw)

	ctx := context.Background()
	capacity := 1
	refillPerSec := 0.5 // one token every 2 seconds

	// Drain the single token.
	ok, _, err := AcquireToken(ctx, db, gw, capacity, refillPerSec)
	if err != nil || !ok {
		t.Fatalf("first acquire failed: err=%v acquired=%v", err, ok)
	}

	before := time.Now()
	ok, nextAvail, err := AcquireToken(ctx, db, gw, capacity, refillPerSec)
	if err != nil {
		t.Fatalf("rate-limited call error: %v", err)
	}
	if ok {
		t.Fatal("expected rate-limited after draining single token")
	}

	// nextAvailable must be strictly after the moment we called AcquireToken.
	if !nextAvail.After(before) {
		t.Errorf("nextAvailable %v is not after call time %v — wait is anchored in the past", nextAvail, before)
	}
	// Wait must be approximately 2 s (1/0.5).  Allow ±500 ms slop.
	wait := time.Until(nextAvail)
	if wait < 1500*time.Millisecond || wait > 2500*time.Millisecond {
		t.Errorf("expected ~2s wait, got %v", wait)
	}
}

// ── 4. Refill: waiting accumulates new tokens ─────────────────────────────────

func TestAcquireToken_RefillOverTime(t *testing.T) {
	db := openTestDB(t)
	gw := "tb_test_refill"
	cleanRateLimit(t, db, gw)

	ctx := context.Background()
	capacity := 2
	refillPerSec := 2.0 // 2 tokens / second

	// Drain both tokens.
	for i := 0; i < capacity; i++ {
		AcquireToken(ctx, db, gw, capacity, refillPerSec) //nolint:errcheck
	}

	// Seed last_refill far enough in the past that 2 tokens should be ready.
	_, err := db.Exec("UPDATE rate_limits SET tokens=0, last_refill=now()-interval '2 seconds' WHERE gateway=$1", gw)
	if err != nil {
		t.Fatalf("seeding last_refill: %v", err)
	}

	// Now acquisition should succeed because 2*2=4 tokens accrued (capped at 2).
	ok, _, err := AcquireToken(ctx, db, gw, capacity, refillPerSec)
	if err != nil {
		t.Fatalf("post-refill acquire: %v", err)
	}
	if !ok {
		t.Error("expected token after refill interval elapsed, got rate-limited")
	}
}

// ── 5. Capacity cap: accrued tokens never exceed capacity ─────────────────────

func TestAcquireToken_CapacityNeverExceeded(t *testing.T) {
	db := openTestDB(t)
	gw := "tb_test_cap"
	cleanRateLimit(t, db, gw)

	ctx := context.Background()
	capacity := 3
	refillPerSec := 10.0

	// Acquire once to create the row.
	AcquireToken(ctx, db, gw, capacity, refillPerSec) //nolint:errcheck

	// Simulate a long idle by winding back last_refill by 10 minutes.
	// Without the cap, tokens would be 3 + 10*60*10 = ~6003. Cap must clamp to 3.
	db.Exec("UPDATE rate_limits SET tokens=0, last_refill=now()-interval '10 minutes' WHERE gateway=$1", gw) //nolint:errcheck

	// Drain capacity tokens — all must succeed.
	for i := 0; i < capacity; i++ {
		ok, _, err := AcquireToken(ctx, db, gw, capacity, refillPerSec)
		if err != nil {
			t.Fatalf("drain call %d: %v", i+1, err)
		}
		if !ok {
			t.Fatalf("drain call %d: expected acquired=true (capacity was %d)", i+1, capacity)
		}
	}

	// Next call must be rate-limited (bucket is now truly empty).
	ok, _, err := AcquireToken(ctx, db, gw, capacity, refillPerSec)
	if err != nil {
		t.Fatalf("post-cap call: %v", err)
	}
	if ok {
		t.Error("expected rate-limited after draining capped bucket, got acquired=true")
	}
}

// ── 6. Concurrent acquisition: FOR UPDATE prevents double-spend ───────────────
//
// 5 goroutines race to acquire from a bucket with capacity=3.
// Exactly 3 must succeed; 2 must be rate-limited.

func TestAcquireToken_ConcurrentSafety(t *testing.T) {
	db := openTestDB(t)
	gw := "tb_test_concurrent"
	cleanRateLimit(t, db, gw)

	ctx := context.Background()
	capacity := 3
	goroutines := 5

	var (
		wg       sync.WaitGroup
		acquired atomic.Int32
		denied   atomic.Int32
	)

	// Pre-fill the bucket to exactly capacity so we control the count.
	db.Exec("INSERT INTO rate_limits (gateway, tokens, last_refill) VALUES ($1, $2, now()) ON CONFLICT (gateway) DO UPDATE SET tokens=$2, last_refill=now()", gw, capacity) //nolint:errcheck

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := AcquireToken(ctx, db, gw, capacity, 1.0)
			if err != nil {
				t.Errorf("goroutine error: %v", err)
				return
			}
			if ok {
				acquired.Add(1)
			} else {
				denied.Add(1)
			}
		}()
	}
	wg.Wait()

	if acquired.Load() != int32(capacity) {
		t.Errorf("acquired=%d, want %d", acquired.Load(), capacity)
	}
	if denied.Load() != int32(goroutines-capacity) {
		t.Errorf("denied=%d, want %d", denied.Load(), goroutines-capacity)
	}
}

// ── 7. Idempotency: gateway name is the primary key; duplicate INSERT is safe ──

func TestAcquireToken_DuplicateGatewayKey(t *testing.T) {
	db := openTestDB(t)
	gw := "tb_test_duplicate"
	cleanRateLimit(t, db, gw)

	ctx := context.Background()
	// Two sequential calls must not error even though both hit the same key.
	for i := 0; i < 2; i++ {
		_, _, err := AcquireToken(ctx, db, gw, 10, 1.0)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}
}
