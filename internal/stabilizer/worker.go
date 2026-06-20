package stabilizer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"log/slog"

	"github.com/IDEA-Amrita/paystable/internal/config"
	"github.com/IDEA-Amrita/paystable/internal/gateway"
)

// 1)Main stabilizer loop
// A)Run starts the stabilizer worker loop. clientFactory should return a GatewayClient for the given gateway name.
func Run(ctx context.Context, db *sql.DB, cfg *config.Config, lag *LagEstimator, clientFactory func(string) gateway.GatewayClient) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	sem := make(chan struct{}, 20) // 1.6: limit to max 20 concurrent polls globally

	for {
		select {
		case <-ctx.Done(): // the caller asked us to stop → log and exit.
			slog.Info("stabilizer stopping")
			return
		case <-ticker.C: // a tick arrived → we run the polling cycle.
		}

		// 1.2: wrap batch query and in_flight update in a single transaction
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			slog.Error("stabilizer: begin tx failed", "error", err)
			continue
		}

		// B)Pull a small batch of pending polls - grabs up to 10 rows whose status='pending' and scheduled_at <= now().
		// 2.2: scope lock with FOR UPDATE OF vp to avoid locking holds table unnecessarily
		rows, err := tx.QueryContext(ctx, `
		SELECT vp.id, vp.txn_id, vp.attempt_number, h.gateway, h.amount, h.created_at
		FROM verification_polls vp
		JOIN holds h ON vp.txn_id = h.txn_id
		WHERE vp.status = 'pending' AND vp.scheduled_at <= now()
		ORDER BY vp.scheduled_at
		LIMIT 10
		FOR UPDATE OF vp SKIP LOCKED
		`)
		if err != nil {
			slog.Error("stabilizer: fetch pending polls failed", "error", err)
			_ = tx.Rollback()
			continue
		}

		// C)Scan rows into a slice - iterates and updates recent poll results in polls
		var polls []struct {
			ID         int64
			TxnID      string
			Attempt    int
			Gateway    string
			HoldAmount int64
			CreatedAt  time.Time
		}
		for rows.Next() {
			var p struct {
				ID         int64
				TxnID      string
				Attempt    int
				Gateway    string
				HoldAmount int64
				CreatedAt  time.Time
			}
			if err := rows.Scan(&p.ID, &p.TxnID, &p.Attempt, &p.Gateway, &p.HoldAmount, &p.CreatedAt); err != nil {
				slog.Error("stabilizer: scan poll failed", "error", err)
				continue
			}
			polls = append(polls, p)
		}
		rows.Close()

		if len(polls) == 0 {
			_ = tx.Rollback()
			continue
		}

		// 1.2: mark status='in_flight' and set started_at=now() inside the same transaction
		var updateErr error
		for _, p := range polls {
			if _, err := tx.ExecContext(ctx, `UPDATE verification_polls SET status='in_flight', started_at=now() WHERE id=$1`, p.ID); err != nil {
				updateErr = err
				break
			}
		}
		if updateErr != nil {
			slog.Error("stabilizer: update status to in_flight failed", "error", updateErr)
			_ = tx.Rollback()
			continue
		}

		if err := tx.Commit(); err != nil {
			slog.Error("stabilizer: commit transaction failed", "error", err)
			continue
		}

		for _, p := range polls {
			p := p
			// 1.6: Acquire semaphore slot before spawning goroutine
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				slog.Info("stabilizer stopping during semaphore acquire")
				return
			}

			// Launch a goroutine per poll for concurrency, isolation and throughput
			go func() {
				defer func() { <-sem }()
				processPoll(ctx, db, cfg, lag, p, clientFactory)
			}()
		}
	}
}

// 2)core per‑poll logic
func processPoll(ctx context.Context, db *sql.DB, cfg *config.Config, lag *LagEstimator, p struct {
	ID         int64
	TxnID      string
	Attempt    int
	Gateway    string
	HoldAmount int64
	CreatedAt  time.Time
}, clientFactory func(string) gateway.GatewayClient) {
	// A)Rate‑limit via token bucket
	acquired, nextAvailable, err := AcquireToken(ctx, db, p.Gateway, 10, 1.0)
	if err != nil {
		slog.Error("AcquireToken error", "error", err, "gateway", p.Gateway)
		// 1.2 safety: reset status back to pending on error so it isn't stuck in_flight
		if _, err2 := db.ExecContext(ctx, `UPDATE verification_polls SET status='pending', error=$1 WHERE id=$2`, "acquire_token_error: "+err.Error(), p.ID); err2 != nil {
			slog.Error("reset poll status to pending failed", "error", err2, "id", p.ID)
		}
		return
	}
	if !acquired {
		jitter := time.Duration(rand.Int63n(int64(time.Second)))
		scheduled := nextAvailable.Add(jitter)
		// 1.2 safety: update status back to pending since it was marked in_flight in Run()
		if _, err := db.ExecContext(ctx, `UPDATE verification_polls SET scheduled_at=$1, error=$2, status='pending' WHERE id=$3`, scheduled, "rate_limited", p.ID); err != nil {
			slog.Error("reschedule rate limited", "error", err, "id", p.ID)
		}
		return
	}

	// C)obtain a gateway client
	client := clientFactory(p.Gateway)
	if client == nil {
		if _, err := db.ExecContext(ctx, `UPDATE verification_polls SET status='failed', error=$1 WHERE id=$2`, "no_client", p.ID); err != nil {
			slog.Error("mark no_client failed", "error", err, "id", p.ID)
		}
		return
	}
	// D)Call the external gateway
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	gatewayStatus, gatewayAmount, raw, err := client.Status(cctx, p.TxnID) //returns gatewayStatus, gatewayAmount, raw JSON, and err
	if err != nil {
		if _, err2 := db.ExecContext(ctx, `UPDATE verification_polls SET status='failed', error=$1, completed_at=now() WHERE id=$2`, err.Error(), p.ID); err2 != nil {
			slog.Error("update poll failed status failed", "error", err2, "id", p.ID)
		}
		// Schedule next attempt if we haven't reached StabilizationN consecutive failures.
		// Since success always terminates the flow immediately, p.Attempt equals the
		// number of consecutive non-success observations so far.
		if p.Attempt < cfg.StabilizationN {
			schedule := lag.ScheduleFor(p.Gateway)
			var delay time.Duration
			idx := p.Attempt - 1 // 0 for attempt 2, 1 for attempt 3, 2 for attempt 4, 3 for attempt 5
			if idx >= 0 && idx < len(schedule.CatchPolls) {
				delay = schedule.CatchPolls[idx]
			} else {
				delay = schedule.FailAfter
			}
			scheduledAt := p.CreatedAt.Add(delay)
			// 1.4: Normalize scheduledAt fallback to UTC
			if scheduledAt.Before(time.Now().UTC()) {
				scheduledAt = time.Now().UTC()
			}
			if _, err3 := db.ExecContext(ctx, `INSERT INTO verification_polls (txn_id, attempt_number, scheduled_at, status) VALUES ($1, $2, $3, 'pending')`, p.TxnID, p.Attempt+1, scheduledAt); err3 != nil {
				slog.Error("schedule next attempt failed", "error", err3)
			}
		} else {
			// StabilizationN consecutive gateway errors — mark hold as FAILED.
			if err := markHoldExhausted(ctx, db, p.TxnID, "polling_exhausted_gateway_error"); err != nil {
				slog.Error("markHoldExhausted failed", "error", err, "txn", p.TxnID)
			}
		}
		return
	}

	// E)Record a successful gateway response
	var rawJSON json.RawMessage
	if raw != nil {
		rawJSON = raw
	}
	if _, err := db.ExecContext(ctx, `UPDATE verification_polls SET gateway_status=$1, gateway_amount=$2, raw_response=$3, completed_at=now(), status='completed' WHERE id=$4`, gatewayStatus, gatewayAmount, rawJSON, p.ID); err != nil {
		slog.Error("update poll success failed", "error", err, "id", p.ID)
		return
	}

	// F) Asymmetric consensus: trust a single success if amount matches (replica cannot lie about success).
	// A replica can only return 'success' if the write primary already committed it.
	// 'failed'/'pending' may be stale replica lag — keep polling until p99.
	if isSuccessStatus(gatewayStatus) {
		if gatewayAmount == p.HoldAmount {
			// Amount matches — conclusive success. Record lag sample and confirm.
			lag.Record(p.Gateway, time.Since(p.CreatedAt))
			if err := finalizeHold(ctx, db, p.TxnID); err != nil {
				slog.Error("finalize hold failed", "error", err, "txn", p.TxnID)
			}
		} else {
			// Gateway says success but amount doesn't match the hold — suspicious.
			// Do NOT confirm. Mark INDETERMINATE so ops can investigate.
			slog.Warn("amount mismatch on success", "txn", p.TxnID,
				"hold_amount", p.HoldAmount, "gateway_amount", gatewayAmount)
			if err := markHoldIndeterminate(ctx, db, p.TxnID, gatewayAmount, p.HoldAmount); err != nil {
				slog.Error("markHoldIndeterminate failed", "error", err, "txn", p.TxnID)
			}
		}
		return
	}

	// not a conclusive success — schedule next attempt if we haven't seen StabilizationN
	// consecutive non-success observations. Since success always terminates immediately,
	// p.Attempt is exactly the consecutive non-success count.
	if p.Attempt < cfg.StabilizationN {
		schedule := lag.ScheduleFor(p.Gateway)
		var delay time.Duration
		idx := p.Attempt - 1
		if idx >= 0 && idx < len(schedule.CatchPolls) {
			delay = schedule.CatchPolls[idx]
		} else {
			delay = schedule.FailAfter
		}
		scheduledAt := p.CreatedAt.Add(delay)
		// 1.4: Normalize scheduledAt fallback to UTC
		if scheduledAt.Before(time.Now().UTC()) {
			scheduledAt = time.Now().UTC()
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO verification_polls (txn_id, attempt_number, scheduled_at, status) VALUES ($1, $2, $3, 'pending')`, p.TxnID, p.Attempt+1, scheduledAt); err != nil {
			slog.Error("schedule next attempt failed", "error", err)
		}
	} else {
		// StabilizationN consecutive non-success observations — mark hold as FAILED.
		if err := markHoldExhausted(ctx, db, p.TxnID, "polling_exhausted_no_consensus"); err != nil {
			slog.Error("markHoldExhausted failed", "error", err, "txn", p.TxnID)
		}
	}
}

// isSuccessStatus returns true when the gateway status means the payment is captured/settled.
func isSuccessStatus(s string) bool {
	switch s {
	case "success", "captured", "completed":
		return true
	}
	return false
}

// markHoldExhausted forcibly marks a hold FAILED after all poll attempts are exhausted.
// It writes a ledger entry and queues an outbox event so downstream consumers are notified.
func markHoldExhausted(ctx context.Context, db *sql.DB, txnID, reason string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var fromStatus sql.NullString
	// Lock the hold row to prevent concurrent finalization.
	if err := tx.QueryRowContext(ctx, `SELECT status FROM holds WHERE txn_id=$1 FOR UPDATE`, txnID).Scan(&fromStatus); err != nil {
		return err
	}
	// If already finalized, do nothing — another path got there first.
	if fromStatus.Valid && (fromStatus.String == "CONFIRMED" || fromStatus.String == "FAILED" || fromStatus.String == "REFUNDED" || fromStatus.String == "INDETERMINATE") {
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `UPDATE holds SET status='FAILED' WHERE txn_id=$1`, txnID); err != nil {
		return err
	}

	// 1.5: use type-safe JSON construction to prevent invalid formatting
	detailBytes, err := json.Marshal(struct {
		Reason string `json:"reason"`
	}{Reason: reason})
	if err != nil {
		return fmt.Errorf("marshal detail failed: %w", err)
	}
	detail := json.RawMessage(detailBytes)

	if _, err := tx.ExecContext(ctx, `INSERT INTO ledger (txn_id, event_type, source, from_status, to_status, detail) VALUES ($1, 'state_transition', 'stabilizer', $2, 'FAILED', $3)`, txnID, fromStatus.String, detail); err != nil {
		return err
	}

	// 1.5: use type-safe JSON construction
	payloadBytes, err := json.Marshal(struct {
		TxnID  string `json:"txn_id"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}{TxnID: txnID, Status: "FAILED", Reason: reason})
	if err != nil {
		return fmt.Errorf("marshal payload failed: %w", err)
	}
	payload := json.RawMessage(payloadBytes)

	idempotency := "evt_" + txnID + "_FAILED_" + reason
	if _, err := tx.ExecContext(ctx, `INSERT INTO outbox (txn_id, event_type, payload, idempotency_key, next_attempt_at) VALUES ($1, 'transaction.FAILED', $2, $3, now())`, txnID, payload, idempotency); err != nil {
		return err
	}
	return tx.Commit()
}

// markHoldIndeterminate marks a hold INDETERMINATE when the gateway reports success
// but the amount does not match the hold amount. This requires manual ops investigation.
func markHoldIndeterminate(ctx context.Context, db *sql.DB, txnID string, gatewayAmount, holdAmount int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var fromStatus sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status FROM holds WHERE txn_id=$1 FOR UPDATE`, txnID).Scan(&fromStatus); err != nil {
		return err
	}
	// Idempotent — skip if already finalized.
	if fromStatus.Valid && (fromStatus.String == "CONFIRMED" || fromStatus.String == "FAILED" || fromStatus.String == "REFUNDED" || fromStatus.String == "INDETERMINATE") {
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `UPDATE holds SET status='INDETERMINATE' WHERE txn_id=$1`, txnID); err != nil {
		return err
	}

	// 1.5: use type-safe JSON construction
	detailBytes, err := json.Marshal(struct {
		Reason        string `json:"reason"`
		GatewayAmount int64  `json:"gateway_amount"`
		HoldAmount    int64  `json:"hold_amount"`
	}{Reason: "amount_mismatch", GatewayAmount: gatewayAmount, HoldAmount: holdAmount})
	if err != nil {
		return fmt.Errorf("marshal detail failed: %w", err)
	}
	detail := json.RawMessage(detailBytes)

	if _, err := tx.ExecContext(ctx, `INSERT INTO ledger (txn_id, event_type, source, from_status, to_status, detail) VALUES ($1, 'state_transition', 'stabilizer', $2, 'INDETERMINATE', $3)`, txnID, fromStatus.String, detail); err != nil {
		return err
	}

	// 1.5: use type-safe JSON construction
	payloadBytes, err := json.Marshal(struct {
		TxnID         string `json:"txn_id"`
		Status        string `json:"status"`
		Reason        string `json:"reason"`
		GatewayAmount int64  `json:"gateway_amount"`
		HoldAmount    int64  `json:"hold_amount"`
	}{TxnID: txnID, Status: "INDETERMINATE", Reason: "amount_mismatch", GatewayAmount: gatewayAmount, HoldAmount: holdAmount})
	if err != nil {
		return fmt.Errorf("marshal payload failed: %w", err)
	}
	payload := json.RawMessage(payloadBytes)

	idempotency := "evt_" + txnID + "_INDETERMINATE"
	if _, err := tx.ExecContext(ctx, `INSERT INTO outbox (txn_id, event_type, payload, idempotency_key, next_attempt_at) VALUES ($1, 'transaction.INDETERMINATE', $2, $3, now())`, txnID, payload, idempotency); err != nil {
		return err
	}
	return tx.Commit()
}

// 3)Helper: finalizeHold
// Called only when a success + amount-match is observed. Always writes CONFIRMED.
func finalizeHold(ctx context.Context, db *sql.DB, txnID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var fromStatus sql.NullString
	// Lock the hold row to prevent concurrent finalization.
	if err := tx.QueryRowContext(ctx, `SELECT status FROM holds WHERE txn_id=$1 FOR UPDATE`, txnID).Scan(&fromStatus); err != nil {
		return err
	}
	// If already finalized, do nothing — idempotent.
	if fromStatus.Valid && (fromStatus.String == "CONFIRMED" || fromStatus.String == "FAILED" || fromStatus.String == "REFUNDED" || fromStatus.String == "INDETERMINATE") {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE holds SET status='CONFIRMED' WHERE txn_id=$1`, txnID); err != nil {
		return err
	}
	detail := json.RawMessage(`{"reason":"success_observed"}`)
	if _, err := tx.ExecContext(ctx, `INSERT INTO ledger (txn_id, event_type, source, from_status, to_status, detail) VALUES ($1, 'state_transition', 'stabilizer', $2, 'CONFIRMED', $3)`, txnID, fromStatus.String, detail); err != nil {
		return err
	}

	// 1.5: use type-safe JSON construction
	payloadBytes, err := json.Marshal(struct {
		TxnID  string `json:"txn_id"`
		Status string `json:"status"`
	}{TxnID: txnID, Status: "CONFIRMED"})
	if err != nil {
		return fmt.Errorf("marshal payload failed: %w", err)
	}
	payload := json.RawMessage(payloadBytes)

	idempotency := "evt_" + txnID + "_CONFIRMED"
	if _, err := tx.ExecContext(ctx, `INSERT INTO outbox (txn_id, event_type, payload, idempotency_key, next_attempt_at) VALUES ($1, 'transaction.CONFIRMED', $2, $3, now())`, txnID, payload, idempotency); err != nil {
		return err
	}
	return tx.Commit()
}
