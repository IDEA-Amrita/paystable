# Paystable — Product Requirements Document

| Field | Value |
|-------|-------|
| **Version** | 1.0 |
| **Status** | Draft |
| **Author** | Samith Reddy Chinni |
| **Created** | 2026-05-20 |
| **Last Updated** | 2026-05-22 |

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Goals & Non-Goals](#3-goals--non-goals)
4. [User Personas](#4-user-personas)
5. [Use Cases](#5-use-cases)
6. [System Architecture](#6-system-architecture)
7. [Functional Requirements](#7-functional-requirements)
8. [API Specification](#8-api-specification)
9. [State Machine](#9-state-machine)
10. [Integration Contract](#10-integration-contract)
11. [Non-Functional Requirements](#11-non-functional-requirements)
12. [Security](#12-security)
13. [Operational Requirements](#13-operational-requirements)
14. [UX Guidance for Integrators](#14-ux-guidance-for-integrators)
15. [Roadmap](#15-roadmap)
16. [Success Metrics](#16-success-metrics)
17. [Risks & Mitigations](#17-risks--mitigations)
18. [Appendix](#18-appendix)

---

## 1. Executive Summary

Paystable is a lightweight, open-source Go daemon that sits between Indian payment gateways (PayU, Razorpay, Cashfree, PhonePe) and merchant applications. It provides payment-reliability guarantees that gateways do not: durable webhook ingestion, multi-observation verification before terminal state, an append-only reconciliation ledger, guaranteed outbound delivery, and a hold-based state machine for inventory reservation.

**Core invariant:** A single signal from a payment gateway is never trusted. Terminal state requires multiple agreeing observations across time.

**One-line thesis:** Never take irreversible actions on unverified failure signals.

---

## 2. Problem Statement

### 2.1 Root Cause

Indian payment gateways share two architectural weaknesses:

1. **Webhooks are best-effort.** They may not arrive, arrive late, arrive out of order, or report incorrect terminal states (e.g., `failure` when the user's bank has already debited).

2. **Verification APIs hit read replicas.** These replicas lag behind the write primary. Razorpay documents that order status can take "a few minutes" to reflect. A verification call made within that window returns stale data.

### 2.2 Failure Scenario [ Real Incident in A well known fest :) ]

| Step | What Happened |
|------|---------------|
| 1 | User pays via PayU for a fest ticket. Bank debits the amount. |
| 2 | PayU sends a `failure` webhook to the merchant app. |
| 3 | Merchant app acts immediately: releases the seat, shows user a failure screen. |
| 4 | Webhook payload is discarded — never persisted. |
| 5 | Manual retry of PayU's verification API returns `failed` (stale replica). |
| 6 | 10 minutes later, PayU's primary reconciles. Payment was successful. |
| 7 | Seat is already sold to another user. Original user is owed a refund. |

**Two failure points:** webhook discarded + verification API returning stale data.

### 2.3 Scale of the Problem

This is not an edge case. Every team running on Indian payment gateways without dedicated payments-reliability infrastructure hits this daily. Most never detect it — they see a "failed" payment, move on, and never know the user was charged.

---

## 3. Goals & Non-Goals

### 3.1 Goals

| ID | Goal | Success Criteria |
|----|------|-----------------|
| G1 | Zero irreversible actions taken on unverified failure signals | 0 false-negative confirmations in production |
| G2 | 100% webhook durability | Every valid webhook persisted before any processing |
| G3 | Multi-observation verification | Terminal state only after N≥3 agreeing consecutive polls |
| G4 | Guaranteed delivery to merchant app | Verified events delivered with at-least-once semantics + idempotency |
| G5 | Inventory hold without merchant-side timers | Hold API manages TTL, verification, and release callbacks |
| G6 | Full audit trail | Every state transition, webhook, and poll logged with timestamps |
| G7 | Single-binary, single-database deployment | No Kafka, Redis, NATS, or external queues |
| G8 | < 15 second typical confirmation latency (success path) | Measured from webhook arrival to CONFIRMED state |

### 3.2 Non-Goals (Phase 1)

| ID | Non-Goal | Rationale |
|----|----------|-----------|
| NG1 | Multi-tenant isolation | Phase 2. Single-merchant deployment for v1. |
| NG2 | Bank statement reconciliation | Phase 2. Requires bank-specific integrations. |
| NG3 | Replacing the payment gateway | Paystable is middleware, not a PSP. |
| NG4 | PCI-DSS card data handling | We never see card numbers. Gateway handles tokenization. |
| NG5 | Automatic refund initiation | We report state; merchant decides refund policy. |
| NG6 | Mobile SDK | Server-side only. Frontend is the merchant's responsibility. |


---

## 4. User Personas

| Persona | Description | Pain Point |
|---------|-------------|------------|
| **Fest Tech Lead** | College student running ticketing for a 10k-attendee fest. No payments engineer. Ships on PayU because it's what the college has. | Gets burned by webhook failures during peak sale. Discovers refund liability days later. |
| **Indie SaaS Founder** | Solo dev running a subscription product on Razorpay. | Sees churn from users whose payments "failed" but were actually charged. Support tickets pile up. |
| **Event Platform CTO** | Runs a multi-event ticketing platform. 50+ events/month. | Needs audit trail for gateway disputes. Currently reconciles manually via spreadsheets. |
| **Ops/Finance Person** | Non-technical. Handles refunds and gateway disputes. | Needs exportable proof that gateway reported X but reality was Y. |

---

## 5. Use Cases

### UC-1: Webhook Arrives, App is Down
1. Gateway fires webhook.
2. Paystable receives, verifies HMAC, persists to postgres.
3. Merchant app is unreachable.
4. Paystable retries delivery via outbox with exponential backoff.
5. App comes back online, receives verified event idempotently.

### UC-2: False Failure Webhook
1. Gateway sends `payment.failed` webhook.
2. Paystable persists it, moves txn to `VERIFYING`.
3. Polls gateway status API: 5s → 10s → 20s.
4. Poll 1: `failed`. Poll 2: `failed`. Poll 3: `success`.
5. Stability broken — counter resets. Polls again.
6. Poll 4: `success`. Poll 5: `success`. Poll 6: `success`.
7. 3 consecutive `success` → txn moves to `CONFIRMED`.
8. Merchant app receives `CONFIRMED` event. Ticket issued.

### UC-3: Genuine Failure
1. Gateway sends `payment.failed` webhook.
2. Paystable persists, moves to `VERIFYING`.
3. Polls: `failed`, `failed`, `failed` (3 consecutive). 
4. Txn moves to `FAILED`. Merchant app notified. Safe to release inventory.

### UC-4: Hold TTL Expiry
1. Merchant calls `POST /hold` with `ttl_seconds: 300`.
2. 300s pass with no terminal signal.
3. Paystable runs one final verification pass before releasing.
4. If final pass returns `success` → `CONFIRMED`. No release.
5. If final pass returns `failed`/`unknown` → `FAILED`. Release callback sent.

### UC-5: Gateway Degradation Detection
1. Paystable detects >40% of polls on a gateway returning stale/inconsistent data.
2. Gateway marked `degraded`.
3. Polling intervals automatically extended (2x multiplier).
4. Alert fired to Slack/Telegram.
5. When consistency recovers, gateway unmarked automatically.

---

## 6. System Architecture

```
┌─────────────┐       ┌─────────────────────────────────────────────┐       ┌─────────────┐
│   Payment   │       │              PAYSTABLE                      |       │  Merchant   │
│   Gateway   │──────>│                                             |──────>│    App      │
│  (PayU etc) │       │  ┌─────────┐  ┌──────────┐  ┌───────────┐   │       │             │
│             │<──────│  │Webhook  │→ │Stabilizer│→ │  Outbox   │   │       │             │
│             │ polls │  │Ingester │  │  Engine  │  │  Delivery │   │       │             │
│             │       │  └─────────┘  └──────────┘  └───────────┘   │       │             │
│             │       │       │            │              │         │       │             │
│             │       │       ▼            ▼              ▼         │       │             │
│             │       │  ┌──────────────────────────────────────┐   │       │             │
│             │       │  │         PostgreSQL                   │   │       │             │
│             │       │  │  webhooks | ledger | outbox | jobs   │   │       │             │
│             │       │  └──────────────────────────────────────┘   │       │             │
│             │       │                                             │       │             │
└─────────────┘       └─────────────────────────────────────────────┘       └─────────────┘
```

**Components:**
- **Webhook Ingester** — HMAC verification, persistence, quarantine for invalid signatures.
- **Stabilizer Engine** — Exponential-backoff poller, jitter, N-of-N consecutive agreement logic.
- **Hold Manager** — TTL tracking, final-verification-before-release.
- **Outbox Delivery** — At-least-once delivery to merchant with idempotency keys.
- **Reconciliation Ledger** — Append-only log of all events and state transitions.
- **Ops Dashboard** — React UI embedded in binary via `embed.FS`.

---

## 7. Functional Requirements

### FR-1: Webhook Ingestion

| ID | Requirement |
|----|-------------|
| FR-1.1 | All inbound webhooks MUST be persisted to postgres before any processing. |
| FR-1.2 | HMAC signature verification MUST occur before persistence to the main ledger. |
| FR-1.3 | Failed-HMAC webhooks MUST be written to a `webhooks_rejected` quarantine table with reason and raw bytes. |
| FR-1.4 | Webhook endpoint MUST return 200 within 5s to prevent gateway retries. Async processing after persist. |
| FR-1.5 | Duplicate webhooks (same event ID) MUST be deduplicated idempotently. |

### FR-2: Verification & Stabilization

| ID | Requirement |
|----|-------------|
| FR-2.1 | No terminal state change from a single observation. Minimum N=3 consecutive agreeing polls required. |
| FR-2.2 | Polling schedule: jittered exponential backoff starting at 5s, capped at 160s. |
| FR-2.3 | Jitter range: ±30% of computed interval. |
| FR-2.4 | If any poll disagrees with the streak, the consecutive counter resets to 0. |
| FR-2.5 | Per-gateway token bucket MUST limit concurrent poll requests (default: 10 req/s per gateway). |
| FR-2.6 | After backoff exhaustion (6 attempts with no stable consensus), txn moves to `INDETERMINATE` and alerts ops. |
| FR-2.7 | Verification MUST check both status AND amount/currency. Amount mismatch = `MISMATCH` state + alert. |

### FR-3: Reconciliation Ledger

| ID | Requirement |
|----|-------------|
| FR-3.1 | Append-only. No UPDATE or DELETE on ledger rows. |
| FR-3.2 | Each entry: `txn_id`, `event_type`, `source` (webhook/poll/manual), `gateway_raw_payload`, `timestamp`, `resulting_state`. |
| FR-3.3 | Exportable as CSV/JSON for gateway dispute resolution. |

### FR-4: Outbound Delivery

| ID | Requirement |
|----|-------------|
| FR-4.1 | Verified events delivered to merchant callback URL with exponential backoff (max 8 retries over 24h). |
| FR-4.2 | Idempotency key = verified-event ID. Merchant receives same key on retries. |
| FR-4.3 | Outbound payloads MUST be HMAC-signed (SHA-256) with merchant's configured secret. |
| FR-4.4 | Delivery considered successful on HTTP 2xx response within 10s. |
| FR-4.5 | Failed deliveries after exhaustion → alert ops, event remains in outbox for manual retry. |

### FR-5: Hold API

| ID | Requirement |
|----|-------------|
| FR-5.1 | `POST /hold` creates a pending transaction with merchant-specified TTL (default 300s, max 900s). |
| FR-5.2 | Idempotent on `txn_id`. Duplicate calls return existing hold, not a new one. |
| FR-5.3 | On TTL expiry: run one final verification pass before emitting release callback. |
| FR-5.4 | If final verification returns success → transition to CONFIRMED, no release. |
| FR-5.5 | Hold MUST reserve the `txn_id` namespace — no other hold can use the same ID. |

### FR-6: State Machine

| ID | Requirement |
|----|-------------|
| FR-6.1 | Legal states: `PENDING`, `VERIFYING`, `CONFIRMED`, `FAILED`, `REFUNDED`, `INDETERMINATE`. |
| FR-6.2 | Terminal states (`CONFIRMED`, `FAILED`, `REFUNDED`) are locked. Only `CONFIRMED` → `REFUNDED` is allowed post-terminal. |
| FR-6.3 | Late webhooks that contradict a terminal state MUST be logged but MUST NOT change state. |
| FR-6.4 | All transitions recorded in the reconciliation ledger. |

### FR-7: Ops Dashboard

| ID | Requirement |
|----|-------------|
| FR-7.1 | Mismatch rate per gateway, over time (chart). |
| FR-7.2 | Full timeline view per transaction. |
| FR-7.3 | Exportable audit reports. |
| FR-7.4 | Slack/Telegram alerts on: mismatch detected, gateway degraded, INDETERMINATE txn, delivery exhaustion. |
| FR-7.5 | Adaptive polling status display per gateway. |


---

## 8. API Specification

### 8.1 Webhook Receiver

```
POST /webhooks/:gateway
Content-Type: application/json
X-Signature: <HMAC signature from gateway>

Body: raw gateway webhook payload
```

**Response:** `200 OK` (always, after persist). No body needed.

### 8.2 Hold API

```
POST /api/v1/hold
Content-Type: application/json
Authorization: Bearer <merchant_api_key>

{
  "txn_id": "order_abc123",
  "gateway": "payu",
  "amount": 49900,
  "currency": "INR",
  "ttl_seconds": 300,
  "callback_url": "https://merchant.app/paystable/callback",
  "metadata": { "seat_id": "A-42", "event": "anokha-2026" }
}
```

**Response (201 Created):**
```json
{
  "txn_id": "order_abc123",
  "status": "PENDING",
  "read_token": "pst_rt_k8x9...",
  "expires_at": "2026-05-22T15:10:00Z",
  "created_at": "2026-05-22T15:05:00Z"
}
```

**Idempotency:** Same `txn_id` returns existing hold with `200 OK`.

### 8.3 Transaction Status

```
GET /api/v1/transactions/:txn_id/status
Authorization: Bearer <merchant_api_key>
```

**Or (public, token-gated for frontend polling):**
```
GET /api/v1/transactions/:txn_id/status?token=<read_token>
```

**Response:**
```json
{
  "txn_id": "order_abc123",
  "status": "VERIFYING",
  "gateway": "payu",
  "amount": 49900,
  "currency": "INR",
  "polls_completed": 2,
  "polls_required": 3,
  "last_poll_result": "success",
  "created_at": "2026-05-22T15:05:00Z",
  "updated_at": "2026-05-22T15:05:12Z"
}
```

### 8.4 Transaction Timeline

```
GET /api/v1/transactions/:txn_id/timeline
Authorization: Bearer <merchant_api_key>
```

**Response:**
```json
{
  "txn_id": "order_abc123",
  "events": [
    { "at": "2026-05-22T15:05:00Z", "type": "hold_created", "source": "api" },
    { "at": "2026-05-22T15:05:03Z", "type": "webhook_received", "source": "payu", "payload_hash": "sha256:ab3f..." },
    { "at": "2026-05-22T15:05:08Z", "type": "poll_result", "source": "payu_api", "result": "success" },
    { "at": "2026-05-22T15:05:18Z", "type": "poll_result", "source": "payu_api", "result": "success" },
    { "at": "2026-05-22T15:05:38Z", "type": "poll_result", "source": "payu_api", "result": "success" },
    { "at": "2026-05-22T15:05:38Z", "type": "state_transition", "from": "VERIFYING", "to": "CONFIRMED" },
    { "at": "2026-05-22T15:05:39Z", "type": "callback_delivered", "target": "https://merchant.app/..." }
  ]
}
```

### 8.5 SSE Stream

```
GET /api/v1/transactions/:txn_id/stream?token=<read_token>
Accept: text/event-stream
```

Emits events on state transitions:
```
event: status_change
data: {"status": "CONFIRMED", "at": "2026-05-22T15:05:38Z"}
```

### 8.6 Secret Rotation

```
POST /api/v1/admin/secrets/rotate
Authorization: Bearer <admin_key>

{
  "gateway": "payu",
  "new_secret": "whsec_new_...",
  "window_hours": 24
}
```

---

## 9. State Machine

```
                    ┌──────────────────────────────────────┐
                    │                                      │
                    ▼                                      │
┌────────┐    ┌──────────┐    ┌───────────┐    ┌────────────────┐
│PENDING │───>│VERIFYING │───>│ CONFIRMED │───>│   REFUNDED     │
└────────┘    └──────────┘    └───────────┘    └────────────────┘
                    │
                    ├──────────> ┌────────┐
                    │            │ FAILED │
                    │            └────────┘
                    │
                    └──────────> ┌───────────────┐
                                 │INDETERMINATE  │
                                 └───────────────┘
```

### Transition Rules

| From | To | Trigger |
|------|----|---------|
| `PENDING` | `VERIFYING` | First webhook received OR first poll initiated |
| `VERIFYING` | `CONFIRMED` | N consecutive polls agree on `success` + amount matches |
| `VERIFYING` | `FAILED` | N consecutive polls agree on `failed` + TTL final-check confirms |
| `VERIFYING` | `INDETERMINATE` | Backoff exhausted, no stable consensus |
| `CONFIRMED` | `REFUNDED` | Refund webhook received + verified |
| `PENDING` | `FAILED` | TTL expires + final verification confirms failure |

### Locked States

- `CONFIRMED`, `FAILED`, `REFUNDED` are terminal (except `CONFIRMED` → `REFUNDED`).
- Late contradicting webhooks are logged to ledger but **do not** alter state.
- `INDETERMINATE` requires manual resolution via admin API or dashboard.

---

## 10. Integration Contract

### 10.1 What the Merchant Must Do

1. **Point gateway webhook URL** at `https://<paystable>/webhooks/:gateway`.
2. **Call `POST /hold`** at checkout start (before redirecting user to gateway).
3. **Implement a callback endpoint** that accepts paystable's signed outbound events.
4. **Poll or SSE** `GET /transactions/:id/status` on the payment-result page.
5. **Verify HMAC** on inbound callbacks from paystable (signature in `X-Paystable-Signature` header).

### 10.2 Callback Payload (Paystable → Merchant)

```
POST <merchant_callback_url>
Content-Type: application/json
X-Paystable-Signature: sha256=<hmac of body with merchant secret>
X-Idempotency-Key: evt_verified_abc123

{
  "event": "transaction.confirmed",
  "txn_id": "order_abc123",
  "status": "CONFIRMED",
  "amount": 49900,
  "currency": "INR",
  "gateway": "payu",
  "verified_at": "2026-05-22T15:05:38Z",
  "metadata": { "seat_id": "A-42", "event": "anokha-2026" }
}
```

### 10.3 Merchant Response Contract

- Return `2xx` within 10 seconds = delivery acknowledged.
- Return `4xx` = permanent failure, no retry (merchant bug).
- Return `5xx` or timeout = transient failure, paystable retries with backoff.

### 10.4 Frontend Integration Pattern

```javascript
// Payment result page
const evtSource = new EventSource(
  `/api/v1/transactions/${txnId}/stream?token=${readToken}`
);

evtSource.addEventListener('status_change', (e) => {
  const { status } = JSON.parse(e.data);
  if (status === 'CONFIRMED') showTicket();
  if (status === 'FAILED') showRetry();
});

// Fallback: poll every 3s if SSE unsupported
```


---

## 11. Non-Functional Requirements

| ID | Category | Requirement |
|----|----------|-------------|
| NFR-1 | Latency | Success-path confirmation ≤ 15s from webhook arrival (typical). Failure-path ≤ 60s. |
| NFR-2 | Throughput | Handle 1000 concurrent holds without degradation on a single 2-core instance. |
| NFR-3 | Availability | Webhook ingestion must survive app-layer restarts (postgres is the durability layer). |
| NFR-4 | Data Retention | Ledger entries retained indefinitely. Webhook raw payloads retained 90 days (configurable). |
| NFR-5 | Deployment | Single static binary. No runtime dependencies beyond postgres. |
| NFR-6 | Startup | Cold start to accepting webhooks < 3 seconds. |
| NFR-7 | Resource | Memory < 128MB RSS under normal load. No goroutine leaks. |
| NFR-8 | Observability | Structured JSON logs. Prometheus metrics endpoint. Health check at `/healthz`. |
| NFR-9 | Graceful Shutdown | On SIGTERM: stop accepting new webhooks, drain in-flight polls (30s max), then exit. |
| NFR-10 | Database | Works with PostgreSQL 14+. Uses `SELECT ... FOR UPDATE SKIP LOCKED` for job queue. |

---

## 12. Security

### 12.1 Inbound (Gateway → Paystable)

| Concern | Mitigation |
|---------|------------|
| Webhook forgery | HMAC verification before persistence. Per-gateway signing scheme support. |
| Replay attacks | Deduplicate on gateway event ID + timestamp window (reject events > 5 min old if timestamp header present). |
| Secret compromise | Zero-downtime rotation with dual-key acceptance window. |
| Rejected webhooks | Quarantined with full payload for forensic review. Never silently dropped. |

### 12.2 Outbound (Paystable → Merchant)

| Concern | Mitigation |
|---------|------------|
| Callback forgery | All outbound callbacks HMAC-signed with merchant-specific secret. |
| Eavesdropping | HTTPS required for callback URLs. Paystable refuses to deliver to HTTP endpoints. |
| Replay | Idempotency key + timestamp in payload. Merchant should reject events > 5 min old. |

### 12.3 API Access

| Concern | Mitigation |
|---------|------------|
| Unauthorized status reads | Public status endpoint gated by per-transaction `read_token` (unguessable, issued at hold time). |
| Admin API access | Separate admin API key. Rate-limited. IP allowlist optional. |
| Transaction ID enumeration | `txn_id` is merchant-provided (opaque string). `read_token` is paystable-generated UUID. Both required for public access. |

### 12.4 Data

| Concern | Mitigation |
|---------|------------|
| Secrets at rest | Webhook secrets and API keys stored encrypted (AES-256-GCM) in postgres. Decrypted in-memory only. |
| PII | Paystable stores gateway payload as-is. Merchant responsible for not sending unnecessary PII in metadata. |
| Backups | Postgres is sole source of truth. Documentation mandates daily `pg_dump` or WAL archiving. |

---

## 13. Operational Requirements

### 13.1 Deployment Modes

| Mode | Description |
|------|-------------|
| Docker | `docker run paystable/paystable` with env vars. |
| Docker Compose | Paystable + Postgres + Dashboard. One command for local dev. |
| Bare binary | `curl -sSL https://get.paystable.dev \| sh`. Systemd unit file provided. |

### 13.2 Configuration (Environment Variables)

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | Yes | Postgres connection string |
| `GATEWAY` | Yes | Gateway identifier (`payu`, `razorpay`, `cashfree`, `phonepe`) |
| `WEBHOOK_SECRET` | Yes | HMAC secret for verifying inbound webhooks |
| `GATEWAY_API_KEY` | Yes | Key for calling gateway's verification API |
| `MERCHANT_CALLBACK_SECRET` | Yes | Secret for signing outbound callbacks |
| `ADMIN_API_KEY` | Yes | Admin endpoint authentication |
| `PORT` | No | HTTP port (default: 8080) |
| `STABILIZATION_N` | No | Consecutive polls required (default: 3) |
| `MAX_BACKOFF_S` | No | Max poll interval in seconds (default: 160) |
| `HOLD_MAX_TTL_S` | No | Maximum allowed TTL (default: 900) |
| `LOG_LEVEL` | No | `debug`, `info`, `warn`, `error` (default: `info`) |

### 13.3 Monitoring & Alerting

| Metric | Alert Threshold |
|--------|-----------------|
| `paystable_webhook_hmac_failures_total` | > 10/min → possible secret mismatch or attack |
| `paystable_verification_mismatches_total` | > 0 → gateway sent wrong signal |
| `paystable_outbox_delivery_failures_total` | > 50 undelivered in 1h → merchant app likely down |
| `paystable_txn_indeterminate_total` | > 0 → requires human intervention |
| `paystable_gateway_degraded` | boolean gauge per gateway |
| `paystable_poll_latency_seconds` | p99 > 5s → gateway API slow |

### 13.4 Database Maintenance

- Run `VACUUM ANALYZE` on ledger tables weekly (or enable autovacuum tuning).
- Partition `webhooks_raw` by month for retention management.
- Index: `txn_id`, `created_at`, `status`, `gateway`.

---

## 14. UX Guidance for Integrators

Paystable does not own the frontend. But integrators need clear guidance on what to show users during each state.

### 14.1 Recommended UI States

| Paystable Status | User Sees | Rationale |
|------------------|-----------|-----------|
| `PENDING` | "Processing your payment..." | Neutral. No commitment either way. |
| `VERIFYING` (webhook was success) | "Payment received. Finalizing your ticket..." | Honest — gateway said success. We're confirming. Green-tinted spinner. |
| `VERIFYING` (webhook was failure) | "Checking with your bank..." | Neutral. No red. Don't scare them. |
| `CONFIRMED` | "Your ticket is ready!" | Ship it. Show QR/download. |
| `FAILED` | "Payment did not go through." | Show retry button. |
| `INDETERMINATE` | "We're sorting this out. You'll get an email within the hour." | Escalation path. |

### 14.2 Critical UX Rules

1. **Disable "Pay Again" while status ∈ {PENDING, VERIFYING}.** Prevents duplicate payments.
2. **Refresh is safe.** Status endpoint is idempotent. Refresh just re-reads current state.
3. **Email/SMS as source of truth.** On CONFIRMED, send ticket via email immediately. The on-screen page is a convenience, not the contract.
4. **60-second ceiling.** If still VERIFYING after 60s, show "safe to leave, we'll email you" message.
5. **SSE primary, 3s polling fallback.** Page updates the instant state changes.


---

## 15. Roadmap

### Phase 1

| Week | Deliverable |
|------|-------------|
| 1–2 | DB schema, migrations, ledger table, webhook ingestion + HMAC verification (PayU) |
| 2–3 | Stabilizer engine: exponential backoff poller, jitter, N-of-N logic, token bucket |
| 3–4 | Hold API: state machine, TTL management, final-verification-before-release |
| 4–5 | Outbound delivery manager: outbox, retries, HMAC signing, idempotency |
| 5–6 | Ops dashboard: React UI, mismatch view, timeline, alerts (Slack/Telegram) |
| 6 | Integration testing, soak test (simulated 1000-txn burst), documentation |

**Exit criteria:** One gateway (PayU), fully functional, deployed on a real fest ticketing system, zero false-negative confirmations over a 7-day soak test.

### Phase 2

| Feature | Priority |
|---------|----------|
| Razorpay adapter | P0 |
| Cashfree adapter | P1 |
| PhonePe adapter | P1 |
| Bank statement reconciliation (CSV upload + auto-match) | P1 |
| Multi-tenant mode (isolated merchant configs, shared infra) | P2 |
| Webhook replay UI (manual re-trigger from dashboard) | P2 |
| Grafana dashboard templates | P2 |
| Kubernetes Helm chart | P3 |

---

## 16. Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| False-negative rate | 0% | Transactions marked FAILED that were actually successful (detected via bank reconciliation) |
| Mismatch detection rate | 100% | All webhook/verification disagreements caught and logged |
| Confirmation latency (success path) | p50 < 10s, p99 < 30s | Time from first webhook to CONFIRMED |
| Outbound delivery success | > 99.9% within 1 hour | Verified events delivered to merchant |
| Webhook durability | 100% | Zero webhooks lost (all persisted or quarantined) |
| Uptime | 99.9% | Webhook ingestion availability |
| Adoption | 5 production deployments within 3 months of open-source launch | GitHub issues, Docker pulls |

---

## 17. Risks & Mitigations

| # | Risk | Impact | Likelihood | Mitigation |
|---|------|--------|------------|------------|
| R1 | Gateway changes webhook signing scheme without notice | HMAC verification breaks, webhooks quarantined | Medium | Monitor quarantine rate. Alert on spike. Dual-parser fallback for known gateway versions. |
| R2 | Gateway rate-limits verification API during peak | Stabilization delayed, user waits longer | High | Per-gateway token bucket. Adaptive degradation mode. Jitter. Backoff cap. |
| R3 | Postgres becomes single point of failure | All state lost | Medium | WAL archiving, daily pg_dump, read replica for dashboard queries. Document backup as mandatory. |
| R4 | Transaction stuck in INDETERMINATE forever | Merchant never gets resolution | Low | Auto-escalation alert. Dashboard shows aging INDETERMINATE txns. Manual resolution API. |
| R5 | Merchant callback endpoint permanently down | Verified events never delivered | Medium | 24h retry window. Alert after 1h. Events remain in outbox indefinitely for manual replay. |
| R6 | Clock skew between paystable and gateway | TTL and timestamp comparisons incorrect | Low | Use monotonic clocks for TTL. NTP required. Log gateway timestamp vs local timestamp for drift detection. |
| R7 | Duplicate payments from impatient users | User charged twice | High | Hold API is idempotent. UX guidance: disable pay-again during VERIFYING. Document prominently. |
| R8 | Open-source adoption without maintenance bandwidth | Security issues unpatched, community trust lost | Medium | Scope phase 1 tightly. Automate dependency updates (Dependabot). Clear SECURITY.md with disclosure process. |

---

## 18. Appendix

### A. Glossary

| Term | Definition |
|------|------------|
| **Terminal state** | A state from which no further automatic transitions occur (CONFIRMED, FAILED, REFUNDED). |
| **Stabilization** | The process of confirming a gateway signal via N consecutive agreeing polls. |
| **Mismatch** | A disagreement between a webhook signal and the verified poll result. |
| **Hold** | A pending transaction reservation with a TTL, managed by paystable. |
| **Outbox** | A postgres table of events awaiting delivery to the merchant app. |
| **Quarantine** | Storage for webhooks that failed HMAC verification. |
| **Read token** | An unguessable token issued at hold creation, used for public status polling. |
| **Adaptive polling** | Automatic adjustment of poll intervals when a gateway is detected as degraded. |

### B. Gateway-Specific Notes

| Gateway | Webhook Signing | Verification API | Known Lag |
|---------|----------------|------------------|-----------|
| PayU | Custom hash (SHA-512 with salt) | `POST /merchant/postBackParam` | Up to 5 min on failure |
| Razorpay | HMAC-SHA256 | `GET /v1/orders/:id` | Documented "few minutes" |
| Cashfree | HMAC-SHA256 | `GET /orders/:order_id` | ~2 min observed |
| PhonePe | SHA-256 + salt checksum | `GET /v3/transaction/:merchantId/:txnId/status` | ~3 min observed |

### C. References

- [Razorpay Webhook Docs](https://razorpay.com/docs/webhooks/)
- [PayU Webhook Integration](https://docs.payu.in/docs/server-to-server-integration)
- [Cashfree Webhooks](https://docs.cashfree.com/docs/webhooks)
- PostgreSQL `SKIP LOCKED` pattern: [Postgres Advisory Locks for Job Queues](https://www.2ndquadrant.com/en/blog/what-is-select-skip-locked-for-in-postgresql-9-5/)

---

*End of document. Thank you for reading.[ik you skipped most of it. but if you didnt thanks man] -samith reddy chinni* 
