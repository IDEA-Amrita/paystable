# Database Schema

PostgreSQL 16+. Single database, no external queues or caches [atleast for now, don't judge me].

Schema is auto-migrated on binary startup. Users never run migrations manually.

---

## Design Decisions

**Why `holds` and not `transactions`?**

Paystable does not own the payment. The gateway does. What paystable owns is the *hold* on a resource (a seat, a subscription slot, an inventory item) while it figures out whether the payment actually went through. The table is named after what it represents in our domain, not the gateway's domain.

**Why a separate `ledger` table instead of updating `holds` in place?**

The holds table stores current state. The ledger stores history. If you only have current state, you cannot answer "what did the gateway claim at 15:05:03 vs what we verified at 15:05:38?" Debugging payment issues requires the full timeline. The ledger is append-only. No UPDATEs, no DELETEs, ever.

**Why no separate jobs table?**

The `outbox` and `verification_polls` tables double as their own job queues using `SELECT ... FOR UPDATE SKIP LOCKED`. A row that needs processing has `status = 'pending'`. A worker picks it up, locks it, processes it, updates the status. One table, one source of truth. This eliminates an entire class of consistency bugs where a job table and a data table disagree.

**Why store raw webhook payloads as JSONB?**

Gateway payload formats change without notice. If we parse into typed columns and the format changes, we lose data. JSONB preserves the original payload exactly as received. We extract what we need into typed columns alongside it for indexing, but the raw payload is the forensic record.

**Why KSUID for read_token?**

Read tokens are exposed to frontends for status polling. They need to be unguessable (unlike sequential IDs) but also time-sortable (useful for debugging and log correlation). KSUIDs give both properties in a URL-safe string. UUIDv4 would work for unguessability but loses time ordering.

**Why partition `webhooks` by month?**

Webhook volume scales linearly with transaction volume. A fest doing 10k transactions in 3 days generates 10k+ webhook rows. After 90 days these are only useful for forensics. Monthly partitions let us drop old partitions cleanly without vacuuming a massive table.

---

## Tables

### holds

The central record. One row per checkout attempt.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | bigint | PK, generated always as identity | Internal ID. Never exposed. |
| txn_id | text | UNIQUE, NOT NULL | Merchant-provided. Opaque string. This is the public identifier. |
| gateway | text | NOT NULL | `payu`, `razorpay`, `cashfree`, `phonepe` |
| status | text | NOT NULL, DEFAULT 'PENDING' | One of: PENDING, VERIFYING, CONFIRMED, FAILED, REFUNDED, INDETERMINATE, MISMATCH |
| amount | bigint | NOT NULL | In smallest currency unit (paise for INR). Bigint, not decimal, to avoid floating point. |
| currency | text | NOT NULL, DEFAULT 'INR' | ISO 4217. |
| read_token | text | UNIQUE, NOT NULL | KSUID. Issued at creation. Used by frontend for status polling. |
| callback_url | text | NOT NULL | Where paystable delivers verified events. |
| metadata | jsonb | DEFAULT '{}' | Merchant-provided context passed through untouched (seat_id, event name, etc). |
| ttl_seconds | int | NOT NULL, DEFAULT 300 | How long to hold before considering release. |
| expires_at | timestamptz | NOT NULL | Computed: created_at + ttl_seconds. Indexed for TTL expiry scanner. |
| created_at | timestamptz | NOT NULL, DEFAULT now() | |
| updated_at | timestamptz | NOT NULL, DEFAULT now() | |

**Indexes:**
- `idx_holds_txn_id` on (txn_id) - unique, primary lookup path
- `idx_holds_status` on (status) WHERE status IN ('PENDING', 'VERIFYING') - partial index for active holds only
- `idx_holds_expires_at` on (expires_at) WHERE status = 'PENDING' - TTL expiry scanner
- `idx_holds_gateway_status` on (gateway, status) - dashboard queries

---

### webhooks

Every valid inbound webhook. Persisted before any processing happens.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | bigint | PK, generated always as identity | |
| txn_id | text | NOT NULL, FK -> holds(txn_id) | Links to the hold this webhook belongs to. |
| gateway | text | NOT NULL | |
| gateway_event_id | text | | Gateway's own event identifier, used for deduplication. |
| event_type | text | NOT NULL | Gateway-specific event name (e.g. `payment.failed`, `payment.captured`). |
| payload | jsonb | NOT NULL | Raw gateway payload, unmodified. |
| received_at | timestamptz | NOT NULL, DEFAULT now() | When paystable received it. |

**Indexes:**
- `idx_webhooks_txn_id` on (txn_id)
- `idx_webhooks_gateway_event_id` on (gateway, gateway_event_id) - deduplication lookup
- `idx_webhooks_received_at` on (received_at) - partition key, retention queries

**Partitioning:** Range partitioned on `received_at` by month. Old partitions dropped after retention period (default 90 days).

---

### webhooks_rejected

Quarantine for webhooks that failed HMAC verification. Never silently dropped.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | bigint | PK, generated always as identity | |
| gateway | text | NOT NULL | |
| rejection_reason | text | NOT NULL | `hmac_mismatch`, `malformed_payload`, `unknown_gateway` |
| headers | jsonb | NOT NULL | Full request headers. Useful for debugging secret rotation issues. |
| raw_body | bytea | NOT NULL | Exact bytes received. Not parsed, not interpreted. |
| source_ip | inet | | For forensics if someone is probing the endpoint. |
| received_at | timestamptz | NOT NULL, DEFAULT now() | |

**Indexes:**
- `idx_rejected_received_at` on (received_at)
- `idx_rejected_gateway` on (gateway)

---

### verification_polls

Every poll attempt against the gateway's status API. Also serves as the job queue for the stabilizer engine.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | bigint | PK, generated always as identity | |
| txn_id | text | NOT NULL, FK -> holds(txn_id) | |
| attempt_number | int | NOT NULL | 1-indexed. Used to compute next backoff interval. |
| status | text | NOT NULL, DEFAULT 'pending' | Job status: `pending`, `in_flight`, `completed`, `failed`. |
| gateway_status | text | | What the gateway returned: `success`, `failed`, `pending`, `not_found`. NULL until completed. |
| gateway_amount | bigint | | Amount the gateway reports. NULL until completed. For amount-mismatch detection. |
| raw_response | jsonb | | Full gateway API response. |
| scheduled_at | timestamptz | NOT NULL | When this poll should fire. Worker picks up rows where scheduled_at <= now(). |
| started_at | timestamptz | | When worker picked it up. |
| completed_at | timestamptz | | When gateway responded. |
| error | text | | If the poll itself failed (timeout, 5xx from gateway, rate limited). |

**Indexes:**
- `idx_polls_job_queue` on (scheduled_at) WHERE status = 'pending' - the SKIP LOCKED job queue
- `idx_polls_txn_id` on (txn_id) - timeline queries

**How the job queue works:**

```sql
SELECT id, txn_id, attempt_number
FROM verification_polls
WHERE status = 'pending' AND scheduled_at <= now()
ORDER BY scheduled_at
LIMIT 10
FOR UPDATE SKIP LOCKED;
```

Worker grabs up to 10 pending polls, locks them, executes them, updates status. Other workers skip locked rows. No coordination needed beyond postgres.

---

### ledger

Append-only audit trail. Every state transition, every significant event, with full context.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | bigint | PK, generated always as identity | |
| txn_id | text | NOT NULL, FK -> holds(txn_id) | |
| event_type | text | NOT NULL | `hold_created`, `webhook_received`, `poll_completed`, `state_transition`, `callback_delivered`, `callback_failed`, `ttl_expired`, `manual_resolution` |
| source | text | NOT NULL | `api`, `webhook`, `stabilizer`, `outbox`, `admin` |
| from_status | text | | Previous state. NULL for initial events. |
| to_status | text | | New state. NULL if event doesn't change state. |
| detail | jsonb | DEFAULT '{}' | Event-specific context. For poll_completed: gateway response. For state_transition: reason. |
| created_at | timestamptz | NOT NULL, DEFAULT now() | |

**Indexes:**
- `idx_ledger_txn_id_created` on (txn_id, created_at) - timeline view per transaction
- `idx_ledger_event_type` on (event_type) WHERE event_type = 'state_transition' - dashboard mismatch queries

**Rules:** No UPDATE. No DELETE. Application layer enforces this. If you need to correct a mistake, append a correction event, don't edit history.

---

### outbox

Events awaiting delivery to the merchant's callback URL. Also its own job queue.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | bigint | PK, generated always as identity | |
| txn_id | text | NOT NULL, FK -> holds(txn_id) | |
| event_type | text | NOT NULL | `transaction.confirmed`, `transaction.failed`, `transaction.refunded`, `hold.released` |
| payload | jsonb | NOT NULL | The full callback body that will be sent to the merchant. |
| idempotency_key | text | UNIQUE, NOT NULL | Derived from: `evt_{txn_id}_{event_type}_{id}`. Merchant uses this to deduplicate. |
| status | text | NOT NULL, DEFAULT 'pending' | `pending`, `in_flight`, `delivered`, `exhausted` |
| attempts | int | NOT NULL, DEFAULT 0 | |
| max_attempts | int | NOT NULL, DEFAULT 8 | 8 retries over ~24h with exponential backoff. |
| next_attempt_at | timestamptz | NOT NULL, DEFAULT now() | |
| last_attempt_at | timestamptz | | |
| last_http_status | int | | Response code from merchant. NULL until first attempt. |
| last_error | text | | Timeout, connection refused, etc. |
| delivered_at | timestamptz | | When merchant returned 2xx. |
| created_at | timestamptz | NOT NULL, DEFAULT now() | |

**Indexes:**
- `idx_outbox_job_queue` on (next_attempt_at) WHERE status = 'pending' - SKIP LOCKED delivery queue
- `idx_outbox_txn_id` on (txn_id)
- `idx_outbox_status` on (status) WHERE status = 'exhausted' - alerting on failed deliveries

---

### gateway_secrets

Webhook signing secrets with zero-downtime rotation support.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | bigint | PK, generated always as identity | |
| gateway | text | NOT NULL | |
| secret_encrypted | bytea | NOT NULL | AES-256-GCM encrypted with `SECRET_ENCRYPTION_KEY`. |
| is_active | boolean | NOT NULL, DEFAULT true | |
| rotation_window_end | timestamptz | | If set, this key is in rotation and will be deactivated after this time. |
| created_at | timestamptz | NOT NULL, DEFAULT now() | |
| deactivated_at | timestamptz | | |

**How rotation works:**

At any point, there can be at most 2 active secrets per gateway (the current one and the rotating-in one). When verifying an inbound webhook, paystable tries both active secrets. After `rotation_window_end` passes, a background job sets `is_active = false` and `deactivated_at = now()` on the old key.

**Indexes:**
- `idx_secrets_gateway_active` on (gateway) WHERE is_active = true

---

## State Transition Enforcement

The application layer enforces legal transitions. The database adds a safety net via a trigger:

```sql
CREATE OR REPLACE FUNCTION enforce_hold_transitions() RETURNS trigger AS $$
BEGIN
  IF OLD.status IN ('CONFIRMED', 'FAILED', 'REFUNDED') THEN
    IF NOT (OLD.status = 'CONFIRMED' AND NEW.status = 'REFUNDED') THEN
      RAISE EXCEPTION 'illegal transition from % to %', OLD.status, NEW.status;
    END IF;
  END IF;
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

This means even if the application has a bug, postgres itself will reject illegal state changes. Belt and suspenders.

---

## Retention and Maintenance

| Table | Retention | Strategy |
|-------|-----------|----------|
| holds | Indefinite | Core business data. Never dropped. |
| webhooks | 90 days (configurable) | Monthly partitions. `DROP PARTITION` for cleanup. |
| webhooks_rejected | 30 days | Simple DELETE by received_at. Low volume. |
| verification_polls | 90 days | DELETE completed polls older than threshold. |
| ledger | Indefinite | Audit trail. Never dropped. |
| outbox | 30 days after delivery | DELETE delivered rows older than threshold. Exhausted rows kept indefinitely for review. |
| gateway_secrets | Indefinite | Low volume. Deactivated rows kept for audit. |
