---
title: Database Schema
description: Current PostgreSQL tables used by Paystable.
---

Paystable uses PostgreSQL for storage and job queues. The schema is auto-migrated on binary startup.

## Tables

| Table | Purpose |
|---|---|
| `holds` | Current state for each checkout attempt. |
| `webhooks` | Valid gateway webhooks. |
| `webhooks_rejected` | Invalid or unverifiable webhook payloads. |
| `verification_polls` | Gateway status checks and stabilizer queue. |
| `ledger` | Append-only state and evidence timeline. |
| `outbox` | Signed merchant callback delivery queue. |
| `gateway_secrets` | Encrypted webhook secrets for rotation. |

## Holds

Important fields:

| Column | Notes |
|---|---|
| `txn_id` | Merchant-supplied unique ID. |
| `status` | `PENDING`, `VERIFYING`, `CONFIRMED`, `FAILED`, `REFUNDED`, `INDETERMINATE`, `MISMATCH`. |
| `amount` | Smallest currency unit. |
| `read_token` | Public status/SSE token. |
| `callback_url` | Merchant backend endpoint. |
| `metadata` | Passed through in callbacks. |
| `expires_at` | TTL deadline for final verification. |

Terminal states are protected by a database trigger. `CONFIRMED`, `FAILED`, `REFUNDED`, `INDETERMINATE`, and `MISMATCH` cannot be automatically changed, except the reserved `CONFIRMED -> REFUNDED` transition.

## Webhooks

`webhooks` intentionally does not keep a foreign key to `holds` in the current migration. This lets Paystable preserve early or out-of-order gateway webhooks even if the hold row is not present yet.

Duplicate gateway events are deduplicated by unique `(gateway, gateway_event_id)`.

The table is not partitioned in the current release.

## Verification Polls

Workers claim pending polls with:

```sql
SELECT ...
FROM verification_polls vp
JOIN holds h ON vp.txn_id = h.txn_id
WHERE vp.status = 'pending' AND vp.scheduled_at <= now()
ORDER BY vp.scheduled_at
LIMIT 10
FOR UPDATE OF vp SKIP LOCKED;
```

Completed polls store gateway status, gateway amount, raw response, and timestamps.

## Ledger

The ledger is append-only. It records webhook receipt, poll results, state transitions, callback events, and admin operations. Correct bad data with another event; do not edit history.

## Outbox

The outbox stores callback payloads and retry state. Workers claim `pending` rows with `SKIP LOCKED`, send signed HTTP callbacks, and move rows to `delivered`, `pending`, or `exhausted`.

## Gateway Secrets

`gateway_secrets` stores AES-GCM encrypted webhook signing secrets when `SECRET_ENCRYPTION_KEY` is configured. During rotation, Paystable can accept both old and new secrets for the configured window.
