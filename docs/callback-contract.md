# Callback Contract

When paystable verifies a payment it delivers the result to the merchant app via a
signed HTTP POST to the `callback_url` supplied at hold creation. This document is
the binding spec for both sides.

---

## Request

```
POST <callback_url>
Content-Type: application/json
X-Paystable-Signature: sha256=<hex>
X-Paystable-Idempotency-Key: <idempotency_key>
X-Paystable-Timestamp: <unix_seconds>
```

### Payload

```json
{
  "txn_id": "order_abc123",
  "event": "transaction.confirmed",
  "status": "CONFIRMED",
  "amount": 49900,
  "currency": "INR",
  "gateway": "payu",
  "verified_at": "2026-05-22T15:05:38Z",
  "metadata": { "seat_id": "A-42", "event": "anokha-2026" }
}
```

| Field | Type | Notes |
|-------|------|-------|
| `txn_id` | string | The merchant-supplied id from `POST /hold`. |
| `event` | string | One of `transaction.confirmed`, `transaction.failed`, `transaction.indeterminate`. |
| `status` | string | `CONFIRMED`, `FAILED`, or `INDETERMINATE`. |
| `amount` | int64 | Paise (INR smallest unit). Same value from `POST /hold`. |
| `currency` | string | ISO 4217, always `INR` for phase 1. |
| `gateway` | string | Gateway that processed the payment. |
| `verified_at` | string | RFC 3339 timestamp of when the state was finalized. |
| `metadata` | object | Passed through unchanged from `POST /hold`. |

---

## Signature verification

Paystable signs every callback so merchants can verify the request originated from
paystable and was not tampered with in transit.

**Algorithm:** HMAC-SHA256 over the raw request body using the value of
`MERCHANT_CALLBACK_SECRET`. The result is hex-encoded and prefixed with `sha256=`.

**Verification (Go reference implementation):**

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

func verify(body []byte, header, secret string) bool {
    if !strings.HasPrefix(header, "sha256=") {
        return false
    }
    expected := make([]byte, 32)
    hex.Decode(expected, []byte(strings.TrimPrefix(header, "sha256=")))
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    return hmac.Equal(mac.Sum(nil), expected)
}
```

Read the raw body **before** any JSON parsing. Parse after verifying. Never trust a
callback without verifying the signature.

---

## Idempotency

Every callback carries `X-Paystable-Idempotency-Key`. The key is stable across
retries for the same event. Paystable delivers with **at-least-once semantics**:
the same event may arrive more than once (e.g., if the merchant returns 2xx but the
connection drops before paystable records it). Merchants must deduplicate on this
key before taking action.

The key format is `evt_<txn_id>_<STATUS>`. Store it and reject duplicates.

---

## Expected responses

| Status | Meaning | Paystable action |
|--------|---------|-----------------|
| `2xx` | Accepted. | Marks delivery complete. No further retries. |
| `4xx` (not 429) | Permanent client error. | Stops retrying immediately. Marks `exhausted`. Ops alerted. |
| `5xx`, `429`, timeout, connection refused | Transient. | Retries with exponential backoff. |

> [!WARNING]
> Review `400 Bad Request` responses from your application carefully. While Paystable treats 4xx codes as permanent errors to stop the retry loop, a `400` response could indicate that your application is rejecting the payload format (e.g. due to a schema validation or parsing mismatch). Check payload formats if you notice unexpected delivery exhaustions.

Respond within **10 seconds**. A slow response counts as a timeout.

---

## Retry schedule

Paystable retries up to 8 times on transient failures, spaced to cover ~24 hours:

| Attempt | Delay after previous |
|---------|---------------------|
| 2 | ~10s |
| 3 | ~1m |
| 4 | ~5m |
| 5 | ~30m |
| 6 | ~2h |
| 7 | ~6h |
| 8 | ~12h |

All delays include full jitter. After attempt 8 the event is marked `exhausted` and
paystable pages ops. The event remains in the outbox for manual replay.

---

## Security requirements

Callback URLs must use HTTPS in production. Paystable refuses to deliver to plain
HTTP endpoints unless `DELIVERY_ALLOW_INSECURE_CALLBACK=true` (local dev only).

---

## Integration checklist

- [ ] Implement the HMAC-SHA256 verification on every inbound callback.
- [ ] Deduplicate on `X-Paystable-Idempotency-Key` before processing.
- [ ] Return `2xx` only after the event has been durably processed.
- [ ] Return `4xx` for malformed requests (wrong signature, unknown event type).
- [ ] Return `5xx` when your app is temporarily unable to process (triggers retry).
- [ ] Respond within 10 seconds.
