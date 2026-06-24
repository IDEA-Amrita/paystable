---
title: Callback Contract
description: Signed final-state callbacks from Paystable to your backend.
---

Paystable sends final outcomes to the hold `callback_url`. Treat this callback as the trusted trigger for fulfillment or release.

## Request

```http
POST <callback_url>
Content-Type: application/json
X-Paystable-Signature: sha256=<hex-hmac>
X-Paystable-Idempotency-Key: <opaque-key>
X-Paystable-Timestamp: <unix-seconds>
```

## Payload

```json
{
  "txn_id": "order_abc123",
  "event": "transaction.confirmed",
  "status": "CONFIRMED",
  "amount": 49900,
  "currency": "INR",
  "gateway": "payu",
  "verified_at": "2026-06-24T12:00:19Z",
  "metadata": {
    "order_id": "order_abc123"
  }
}
```

Review states can include `reason`, `gateway_amount`, and `hold_amount`.

| Field | Notes |
|---|---|
| `event` | `transaction.confirmed`, `transaction.failed`, `transaction.indeterminate`, or `transaction.mismatch`. |
| `status` | `CONFIRMED`, `FAILED`, `INDETERMINATE`, or `MISMATCH`. |
| `amount` | Hold amount, in smallest currency unit. |
| `metadata` | Original hold metadata. |

## Verify Signature

The signature is HMAC-SHA256 over the raw request body using `MERCHANT_CALLBACK_SECRET`.

```js
import crypto from "node:crypto";

export function verifyPaystableCallback(rawBody, header, secret) {
  if (!header?.startsWith("sha256=")) return false;

  const received = Buffer.from(header.slice("sha256=".length), "hex");
  const expected = crypto
    .createHmac("sha256", secret)
    .update(rawBody)
    .digest();

  return received.length === expected.length &&
    crypto.timingSafeEqual(received, expected);
}
```

## Idempotency

Paystable delivers at least once. Store `X-Paystable-Idempotency-Key` before taking irreversible action. If the same key arrives again, return `2xx` and skip processing.

Treat the key as opaque.

## Retry Behavior

| Merchant response | Paystable action |
|---|---|
| `2xx` | Mark delivered. |
| `4xx` except `429` | Mark exhausted. |
| `429`, `5xx`, timeout | Retry with backoff. |

Default timeout: `DELIVERY_TIMEOUT_S=10`.

Production callback URLs must be HTTPS unless `DELIVERY_ALLOW_INSECURE_CALLBACK=true` is set for local development.
