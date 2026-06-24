# Lag Estimator

The lag estimator chooses when Paystable should schedule verification polls for a gateway. It does not replace the state machine. Terminal success or failure still depends on the stabilizer's status and amount checks.

Implementation: `internal/stabilizer/lag.go`.

## Why it exists

Flat polling is easy:

```text
5s -> 10s -> 20s -> 40s -> 80s -> 160s
```

But gateways do not all become consistent at the same speed. A schedule that is too aggressive wastes calls and can keep hitting stale data. A schedule that is too slow makes honest success feel broken to the user.

The estimator keeps recent per-gateway samples and uses them to choose "catch polls" and a failure deadline.

## What gets sampled

Only confirmed transactions produce lag samples.

```text
lag_sample = time first success was observed - hold creation time
```

That keeps failed transactions from polluting the distribution. A true failure may never produce success, so including it as a lag sample would make the model meaningless.

## How scheduling works

For each gateway, the estimator returns:

- catch poll targets around recent p50, p75, and p90 success lag
- a conservative fail-after target around p99
- a cold-start prior when there are too few samples

The stabilizer then schedules attempts from that model. The current release still requires `STABILIZATION_N` matching completed polls before a terminal success or failure. The estimator improves *when* those polls happen; it does not make one poll magically final.

## What it buys

- Faster confirmation when a gateway is normally quick.
- More patience when a gateway is currently lagging.
- Less pressure on gateway status APIs during bursts.
- A cleaner path to gateway-specific behavior without hardcoding every timeout.

## Important caveat

Success must mean captured or settled, not merely authorized. The current success path also checks the gateway amount against the hold amount. A success status with the wrong amount becomes `MISMATCH`, not `CONFIRMED`.
