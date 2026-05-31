# Lag Estimator

## What it is

The lag estimator is a small, per-gateway model that learns how long a genuinely
successful payment takes to show up on the gateway's verification API after the
webhook arrives. The stabilizer uses it to decide *when* to poll and *when* it is
finally safe to declare a payment failed.

It lives in `internal/stabilizer/lag.go`.

## Why flat polling is not good enough

The first design for the stabilizer was the obvious one: poll the gateway on a
fixed exponential backoff (5s, 10s, 20s, 40s, 80s, 160s) and require N consecutive
agreeing results before calling a transaction final. It is simple and it works,
but it has two real weaknesses.

**It treats success and failure the same.** Read replicas lag *behind* the write
primary. A replica cannot report `success` unless the primary already committed
it, so a single `success` observation is essentially conclusive. A `failed` or
`pending` observation is not: it might be a true failure, or it might be a real
success that has not propagated to the replica yet. Spending the same number of
polls on both cases wastes time confirming the easy case (success) and provides
false comfort on the hard case (failure).

**Re-polling the same lagging replica is not independent evidence.** N-of-N
agreement feels strong because three matching observations sounds like compounding
confidence. That only holds if the observations are independent. If all three
polls happen to hit the same replica that is still serving a stale `failed`, you
have three agreements that collectively carry the information of one. The
confidence is an illusion, and it points in the most dangerous direction:
prematurely declaring a successful payment as failed.

## An illustrative scenario

Consider a single PayU transaction to make the failure mode concrete. (This is an
illustration of the mechanism, not a log of a specific incident.)

1. A customer pays. Their bank debits the amount. PayU's write primary records the
   payment as successful.
2. Before that success propagates to PayU's read replicas, PayU fires a webhook.
   Due to internal ordering, the webhook carries a `failure` status.
3. Paystable persists the webhook and begins verifying. It polls the status API.
4. The first poll is routed to a replica that has not yet caught up. It returns
   `failed`. So does the second poll a few seconds later, because the load balancer
   pinned the connection to the same replica.
5. Under flat N-of-2, that is two agreeing observations. The transaction would be
   declared `FAILED`, the seat released, and the customer, who has been charged,
   would be told their payment did not go through. This is exactly the class of bug
   paystable exists to prevent.

The problem is not that polling is wrong. It is that the schedule and the stopping
rule were not tied to how long this particular gateway actually takes to become
consistent. The right amount of patience is a property of the gateway, and it can
be measured.

## How the estimator works

The core idea: paystable eventually learns the ground truth for every transaction,
because it keeps verifying until the state is stable. So it can look back and
measure, per gateway, how long real successes took to appear.

**Condition on success.** For every transaction that ended `CONFIRMED`, record one
sample:

```
lag_sample = (time of first poll that returned success) - (webhook arrival time)
```

Only confirmed transactions are sampled. Truly failed transactions never emit a
success signal, so including them would censor the distribution and push every
estimate too high. Conditioning on success is the key correctness point.

**Drive the schedule from the distribution.** Given the recent lag samples for a
gateway, the estimator computes quantiles:

- Catch polls at **p50, p75, p90** so real successes are confirmed as early as the
  data says they usually appear.
- A failure deadline at **p99**. If a true success would almost always have shown
  up by the 99th percentile of historical lag, and none has, the transaction is
  overwhelmingly likely to be genuinely failed. That is the stopping bound, derived
  from data instead of guessed.

**Stay adaptive.** Samples are kept in a bounded ring buffer (most recent 500). If
a gateway slows down, recent samples rise, the quantiles rise, and polling spaces
itself out automatically. The "degraded gateway" behaviour described in the PRD
falls out of this for free, with no special casing.

**Handle cold start.** With fewer than 50 samples for a gateway, the empirical
quantiles cannot be trusted, so the estimator falls back to a conservative prior
(p50 10s, p75 30s, p90 60s, p99 180s) that reflects the documented "few minutes"
worst case. As real samples accumulate, it switches to the gateway's own measured
distribution.

## What this buys us

- **Faster on the common path.** A real success is confirmed on the first poll that
  observes it, rather than waiting out a fixed N-of-N count.
- **Honest on the failure path.** Failure is only declared after the gateway's own
  measured propagation window has elapsed, not after an arbitrary fixed number of
  possibly-correlated polls.
- **Self-tuning per gateway.** PayU, Razorpay, Cashfree and PhonePe each get their
  own learned timing, and each adapts as conditions change.

## Caveat: success must mean captured

"A single success is conclusive" assumes the gateway's `success` means the payment
is captured/settled, not merely authorized. In auth-then-capture flows an
authorization can still be voided. The stabilizer therefore confirms on success
only when the amount matches and the payment is actually captured, not on the
status string alone.
