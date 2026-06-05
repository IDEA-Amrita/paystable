# Implementation Plan — Stabilizer: backoff, jitter, N-of-N, token-bucket

Goal
----
Add a database-backed stabilizer engine that reliably verifies gateway transaction state by:
- Retrying gateway status checks with exponential backoff (capped) and jitter.
- Coordinating requests across processes with a DB token-bucket rate limiter.
- Declaring terminal state only when the last N verification polls agree (N-of-N).

Principles
----------
- Non-destructive: do not delete or replace existing files. All changes are additive.
- DB-first job queue: use `verification_polls` as the job queue (already in `docs/schema.md`).
- Idempotent work: workers use `SELECT ... FOR UPDATE SKIP LOCKED` and update rows so retries are safe.
- Conservative defaults: `STABILIZATION_N=3`, `MAX_BACKOFF_S=160`, `HOLD_MAX_TTL_S=900`.

Scope / Deliverables
--------------------
Files to ADD (primary implementation):

- `internal/stabilizer/backoff.go` — compute base delay for an attempt, apply jitter.
- `internal/stabilizer/tokenbucket_db.go` — DB-backed token-bucket implementation (AcquireToken).
- `internal/stabilizer/worker.go` — stabilizer worker loop: pick polls, enforce rate-limit, call gateway, update polls, schedule next attempts, run N-of-N checks and apply state transitions.
- `internal/gateway/client.go` — small gateway client interface used by worker.
- `internal/gateway/payu/client.go` — implementation of the gateway client for PayU (status endpoint).
- `internal/database/migrations/002_rate_limits.up.sql` — SQL migration to create `rate_limits` table.

Files to MODIFY (small, additive changes):

- `internal/hold/store.go` — enqueue initial `verification_polls` row after `Create` succeeds.
- `internal/webhook/handler.go` — after `persist`, optionally enqueue an immediate verification poll for the txn to accelerate stabilization.
- `cmd/paystable/main.go` — start stabilizer worker(s) on startup.

Database changes
----------------
Add a small migration to create a per-gateway rate limiter table used by the token-bucket:

```sql
BEGIN;
CREATE TABLE IF NOT EXISTS rate_limits (
  gateway text PRIMARY KEY,
  tokens double precision NOT NULL,
  last_refill timestamptz NOT NULL
);
COMMIT;
```

Backoff algorithm (precise)
---------------------------
Inputs:
- `attempt` (int, 1-indexed)
- `catchPolls = LagEstimator.ScheduleFor(gateway).CatchPolls` (slice of 3 durations: p50,p75,p90)
- `maxBackoff = cfg.MaxBackoffS` (seconds)

Algorithm (in words):

1. If `attempt <= len(catchPolls)` then base = `catchPolls[attempt-1]`.
2. Else compute expoStep = attempt - len(catchPolls) (1-indexed after catch list). Base = min(maxBackoff, 2^(expoStep-1) * 1s).
3. Apply full jitter: delay = uniform_random(0, base).

Token-bucket (DB-backed) design
-------------------------------
Purpose: share a per-gateway rate limit across multiple worker processes and nodes.

Table: `rate_limits(gateway text PRIMARY KEY, tokens double precision, last_refill timestamptz)`

Parameters (configurable):
- capacity (default 10 tokens)
- refill_rate (tokens per second, default 1 token/sec)

AcquireToken algorithm (atomic, inside TX):

1. SELECT tokens, last_refill FROM rate_limits WHERE gateway = $1 FOR UPDATE.
   - If no row, INSERT default row `(gateway, capacity, now())` and allow (tokens--).
2. elapsed = now() - last_refill (seconds float)
3. accrued = elapsed * refill_rate
4. tokens = min(capacity, tokens + accrued)
5. If tokens >= 1: tokens -= 1; update tokens,last_refill; commit; return acquired=true
6. Else compute wait_seconds = (1 - tokens) / refill_rate; next_available = last_refill + wait_seconds; return acquired=false, next_available

Worker behavior on rate-limit failure: update the poll row's `scheduled_at = next_available + jitter` and leave `status='pending'`.

Worker main loop (pseudocode)
-----------------------------
Loop:
  rows = SELECT id, txn_id, attempt_number, gateway FROM verification_polls
         WHERE status='pending' AND scheduled_at <= now()
         ORDER BY scheduled_at LIMIT 10 FOR UPDATE SKIP LOCKED;
  For each row:
    - Attempt AcquireToken(gateway):
        * if !acquired: set scheduled_at = nextAvailable + jitter, set error='rate_limited', set status='pending', continue
    - Call gateway client: `Status(ctx, txn_id)` with timeout (e.g., 10s)
        * on network/5xx/timeouts: record `error`, set `status='failed'` (or keep pending), compute next attempt time with `NextDelay`
        * on success: update `verification_polls` with `gateway_status`, `gateway_amount`, `raw_response`, `completed_at`, `status='completed'`
    - Run N-of-N check (see below). If consensus reached: in a single transaction update `holds.status`, append `ledger` row, insert `outbox` row.
    - If not stabilized and attempt < max_attempts and cumulative < HoldMaxTTLS: insert new verification_polls row with attempt+1 and scheduled_at = now() + NextDelay(attempt+1).

N-of-N stabilization (precise)
-----------------------------
When a poll completes successfully, the worker will check the most recent `N = cfg.StabilizationN` completed poll results for that `txn_id`:

```sql
SELECT gateway_status
FROM verification_polls
WHERE txn_id = $1 AND gateway_status IS NOT NULL
ORDER BY completed_at DESC
LIMIT $N
```

If the query returns exactly `N` rows and all values are equal (e.g., all `success` or all `failed`):

1. Begin TX.
2. Verify current `holds.status` is not already terminal. If not terminal, `UPDATE holds SET status = mappedStatus WHERE txn_id = $1`.
3. INSERT INTO ledger(txn_id, event_type='state_transition', source='stabilizer', from_status, to_status, detail)
4. INSERT INTO outbox(txn_id, event_type, payload, idempotency_key, next_attempt_at) VALUES (...)
5. Commit.

Mapping: `gateway_status` -> `holds.status` e.g., `success` → `CONFIRMED`, `failed` → `FAILED`, otherwise `INDETERMINATE`.

If TTL expires before consensus
--------------------------------
- If the elapsed time since hold creation > `HoldMaxTTLS`, stop scheduling retries and mark the hold `INDETERMINATE`. Append ledger entry and outbox event explaining indeterminate result.

Enqueuing initial polls
------------------------
- In `internal/hold/store.go`, after a successful `Create(...)` insert one `verification_polls` row with `attempt_number = 1` and `scheduled_at = now()` (or small jittered delay).
- In `internal/webhook/handler.go`, after `persist(...)` insert a quick verification poll for same txn to accelerate stabilization when webhook arrives.

Gateway client interface
------------------------
Add `internal/gateway/client.go`:

```go
type GatewayClient interface {
  Status(ctx context.Context, txnID string) (gatewayStatus string, gatewayAmount int64, raw json.RawMessage, err error)
}
```

Implement a PayU client in `internal/gateway/payu/client.go` which performs the HTTP call, respects timeouts, extracts normalized `gatewayStatus` values, and returns raw JSON.

Tests & validation
------------------
- Unit tests:
  - `backoff_test.go`: deterministic checks for base delay computation and jitter distribution bounds.
  - `tokenbucket_db_test.go`: concurrent acquisition simulation and next-available calculation.
  - `stabilizer_nofn_test.go`: check N-of-N detection from in-memory rows or test DB.
- Integration test:
  - Start a test Postgres, run migrations, create a hold, seed fake gateway responses (mock `GatewayClient`), run worker, assert `holds.status` transitions to `CONFIRMED` after N identical responses.

Deployment & rollout
--------------------
- Migration: apply `002_rate_limits.up.sql` on deploy.
- Start worker as part of the main process in `cmd/paystable/main.go` — start 1 worker initially.
- Tunables via env: `STABILIZATION_N`, `MAX_BACKOFF_S`, `HOLD_MAX_TTL_S`.
- Start with conservative rate bucket (capacity small) and monitor rate-limited events.

Monitoring & metrics (suggested)
-------------------------------
- Emit counters for: poll_attempts, poll_successes, poll_failures, rate_limited_events, consensus_reached, consensus_failed.
- Track average attempts-to-consensus and percentage of holds `INDETERMINATE` after TTL.

Safety & Edge Cases
-------------------
- Worker must be idempotent — always update the `verification_polls` row it is working on so other workers skip it.
- Use DB transactions for state transitions to avoid races between two workers detecting consensus simultaneously.
- Choose full jitter to avoid synchronized retries.

Phased rollout (recommended)
---------------------------
1. Add DB migration and `token_limits` table.  
2. Add `backoff.go` and `tokenbucket_db.go` and unit tests.  
3. Add worker implementation and gateway clients but start worker disabled by config. Deploy and run unit/integration tests.
4. Enable worker in production with low concurrency and monitor.
5. Increase concurrency or workers when stable.

Appendix: quick NextDelay pseudocode
-----------------------------------

```go
func NextDelay(attempt int, catch []time.Duration, maxBackoff time.Duration) time.Duration {
  if attempt <= len(catch) {
    base := catch[attempt-1]
    return time.Duration(rand.Int63n(int64(base))) // full jitter
  }
  expoStep := attempt - len(catch)
  base := time.Second << (expoStep-1)
  if base > maxBackoff { base = maxBackoff }
  return time.Duration(rand.Int63n(int64(base)))
}
```

---
End of plan. Everything above is additive and intended to slot into the existing `verification_polls` job queue design in `docs/schema.md`.
