# paystable

> **never take irreversible actions on unverified failure signals.**

a tiny open-source Go daemon that sits between your Indian payment gateway and your app, and gives you the reliability guarantees the gateway doesn't.

you don't swap your gateway. you don't rewrite your checkout. you put paystable in the middle, point your gateway's webhook at it, and let it do the boring, brutal work of figuring out what *actually* happened to a payment.

---

## the problem

every Indian payment gateway — payu, razorpay, cashfree, phonepe — ships the same two bugs:

**webhooks are best-effort.** sometimes they don't arrive. sometimes they arrive late. sometimes they fire a `failure` while the user's bank has already debited the money.

**verification APIs lie.** they hit read replicas that lag the write primary. razorpay literally documents that order status can take *"a few minutes"* to reflect. you ask "did this payment succeed?" and the gateway confidently says "no" — when in fact, yes, the money is moving, just not on this replica yet.

put those together and you get the canonical Indian-gateway disaster:

1. failure webhook arrives.
2. you act on it. release the seat. void the order. show the user a red screen.
3. ten seconds later, the truth catches up. payment was actually successful.
4. you now owe a refund you didn't plan for. the user is angry. inventory is sold to someone else.
5. you only find out because someone went back and checked manually.

we got burned by exactly this at **a well known fest :)** (PayU + ticketing). a webhook the gateway sent us was discarded entirely — never persisted. the verification API we fell back to was hitting a stale replica. two failure points, one furious user, one refund out of pocket.

every college fest, indie SaaS, and event platform on these rails hits this. most never realise.

---

## the fix

paystable enforces one rule:

> a single signal from your gateway is **never** trusted. terminal state requires multiple agreeing observations across time.

that's the whole project.

---

## how it works

six layers, all in one binary, all backed by postgres:

### 1. webhook ingestion
every inbound webhook is HMAC-verified first (razorpay = HMAC-SHA256, payu = its own scheme). signature mismatch → quarantined to a separate table, never touches the ledger. signature ok → persisted before anything else runs. your app can be down when the webhook arrives — paystable has it, and will replay it when you come back.

### 2. verification & stabilization
a failure webhook is **never** acted on directly. paystable queues it and polls the gateway's status API on jittered exponential backoff:

```
5s → 10s → 20s → 40s → 80s → 160s
```

the status must be **stable across N consecutive checks** (default `N=3`) before it's marked verified. one API call is a guess. three agreeing API calls is the truth. jitter prevents 100 failed checkouts at the same minute from thundering payu's status endpoint into a rate-limit ban.

### 3. reconciliation ledger
permanent, append-only record of every webhook, every poll, every state transition, with timestamps and gateway raw payloads. when something goes wrong, you stop saying *"something went wrong"* and start saying *"here is exactly what went wrong, signed, dated, exportable."* useful for refund decisions, internal audits, and disputes with the gateway when their numbers and yours disagree.

### 4. outbound delivery manager
paystable knowing the truth is half the job. the other half is making sure your app receives it. we maintain a postgres outbox. every verified event is delivered to your app with retries, exponential backoff, and idempotency keys (keyed on the verified-event id, so a duplicate delivery is a no-op). your app can be down for an hour and not lose a single confirmation.

### 5. hold API
your checkout flow calls:

```http
POST /hold
{ "txn_id": "...", "ttl_seconds": 300 }
```

paystable holds the record in `PENDING`. if verification confirms success inside the TTL, the hold flips to `CONFIRMED`. if the TTL expires, paystable runs **one final verification pass** before sending the release callback to your app — because a TTL alone is exactly the kind of unverified signal we refuse to act on. you don't manage timers, retries, or state machines. paystable does.

### 6. ops dashboard
a small React UI showing:
- mismatch rate per gateway, over time
- full timeline of every mismatch transaction
- exportable audit reports for gateway disputes
- slack / telegram alerts the *moment* a mismatch is detected
- **adaptive polling** — if paystable detects a gateway is consistently returning stale reads, it marks it `degraded` and automatically slows polling across every active txn on that gateway. your built-in answer to rate limits.

---

## integrating with your app

one endpoint:

```http
GET /transactions/:id/status
```

returns:

| status      | meaning                                                                 |
|-------------|-------------------------------------------------------------------------|
| `PENDING`   | hold created, no terminal signal yet.                                   |
| `VERIFYING` | gateway claimed something. paystable is stabilizing. don't act.         |
| `CONFIRMED` | money is in across multiple agreeing polls. ship it.                    |
| `FAILED`    | confirmed failure, stable. safe to refund / release.                    |
| `REFUNDED`  | post-confirmation reversal.                                             |

your frontend opens an SSE stream (or polls every 3s as fallback) on the payment-result page. the user can refresh, close the tab, come back tomorrow — paystable answers consistently. what you *show* the user during each state is your problem. keeping the status accurate is ours.

---

## quickstart

```bash
curl -sSL https://paystable.vercel.app | sh
cp .env.example .env
# fill in DATABASE_URL, GATEWAY, API keys
./paystable
# dashboard at http://localhost:8080/dashboard
```

that's it. single static Go binary. no JVM, no Node runtime, no Python. one process, one database, accepting webhooks. dashboard live.

for local development with postgres included:

```bash
docker compose up
```

---

## secret rotation, zero downtime

```bash
paystable secret rotate --new=NEW_SECRET --window=24h
```

paystable accepts webhooks signed by either the old or new key for the duration of the window. when the window closes (auto, with a T-1h alert), the old key is dropped. no 2am pages.

---

## tech stack

- **Go** — single static binary, deploys anywhere
- **PostgreSQL** — ledger, webhook store, outbox, *and* the job queue via `SELECT … FOR UPDATE SKIP LOCKED`
- **React + Tailwind** — dashboard, embedded into the Go binary
- **Docker Compose** — local dev in one command
- **no** Kafka. **no** Redis. **no** NATS. one less moving part is one less thing to break at midnight in the middle of a fest.

---

## roadmap

**phase 1 — 40 days, payu only**
schema + ledger → HMAC verification → exponential stabilizer → hold API state machine → outbound delivery manager → dashboard + soak test.
ship one gateway, correctly.

**phase 2**
razorpay, cashfree, phonepe. bank-statement reconciliation. multi-tenant mode for agencies running paystable on behalf of multiple merchants.

---

## who this is for

college fests. indie SaaS. event ticketing platforms. anyone running on Indian payment gateways without a dedicated payments-reliability engineer on staff.

if you've ever stared at a "failed" payment that wasn't actually failed, refunded a user out of your own pocket, or sold the same seat twice because a webhook lied to you — paystable is for you.

razorpay and payu have no official SDK that handles webhook durability + verification stabilization + reconciliation together. every small team either reinvents this badly, or doesn't reinvent it and gets quietly burned month after month.

we're building the thing we wish existed the day anokha shipped.

---

## docs

- [Product Requirements Document](docs/prd.md) - full PRD with API spec, state machine, security model, and UX guidance
- [Database Schema](docs/schema.md) - every table, column, index, and the reasoning behind each design choice
- [Lag Estimator](docs/lag-estimator.md) - how paystable learns per-gateway verification lag to confirm fast and fail honestly
- [Callback Contract](docs/callback-contract.md) - the binding spec for outbound delivery to merchant apps (headers, signing, idempotency, retry)

---

## license

MIT. take it, run it, fork it, ship it.

## contributing

PRs welcome. gateway-reliability war stories *very* welcome — open an issue with the dump and we'll likely turn it into a test case.

---

*paystable: because "the gateway said it failed" is not the same as "it actually failed."*
