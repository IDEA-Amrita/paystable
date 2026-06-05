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

// Run starts the stabilizer worker loop. clientFactory should return a GatewayClient for the given gateway name.
func Run(ctx context.Context, db *sql.DB, cfg *config.Config, lag *LagEstimator, clientFactory func(string) gateway.GatewayClient) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("stabilizer stopping")
			return
		case <-ticker.C:
		}

		// Fetch a small batch of pending polls and lock them
		rows, err := db.QueryContext(ctx, `
		SELECT vp.id, vp.txn_id, vp.attempt_number, h.gateway
		FROM verification_polls vp
		JOIN holds h ON vp.txn_id = h.txn_id
		WHERE vp.status = 'pending' AND vp.scheduled_at <= now()
		ORDER BY vp.scheduled_at
		LIMIT 10
		FOR UPDATE SKIP LOCKED
		`)
		if err != nil {
			slog.Error("stabilizer: fetch pending polls failed", "error", err)
			continue
		}
		var polls []struct {
			ID      int64
			TxnID   string
			Attempt int
			Gateway string
		}
		for rows.Next() {
			var p struct {
				ID      int64
				TxnID   string
				Attempt int
				Gateway string
			}
			if err := rows.Scan(&p.ID, &p.TxnID, &p.Attempt, &p.Gateway); err != nil {
				slog.Error("stabilizer: scan poll failed", "error", err)
				continue
			}
			polls = append(polls, p)
		}
		rows.Close()

		for _, p := range polls {
			// process each poll concurrently
			go processPoll(ctx, db, cfg, lag, p, clientFactory)
		}
	}
}

func processPoll(ctx context.Context, db *sql.DB, cfg *config.Config, lag *LagEstimator, p struct {
	ID      int64
	TxnID   string
	Attempt int
	Gateway string
}, clientFactory func(string) gateway.GatewayClient) {
	// token-bucket acquire
	acquired, nextAvailable, err := AcquireToken(ctx, db, p.Gateway, 10, 1.0)
	if err != nil {
		slog.Error("AcquireToken error", "error", err, "gateway", p.Gateway)
		return
	}
	if !acquired {
		jitter := time.Duration(rand.Int63n(int64(time.Second)))
		scheduled := nextAvailable.Add(jitter)
		if _, err := db.ExecContext(ctx, `UPDATE verification_polls SET scheduled_at=$1, error=$2 WHERE id=$3`, scheduled, "rate_limited", p.ID); err != nil {
			slog.Error("reschedule rate limited", "error", err, "id", p.ID)
		}
		return
	}

	// mark in-flight
	if _, err := db.ExecContext(ctx, `UPDATE verification_polls SET status='in_flight', started_at=now() WHERE id=$1`, p.ID); err != nil {
		slog.Error("set in_flight failed", "error", err, "id", p.ID)
		return
	}

	client := clientFactory(p.Gateway)
	if client == nil {
		if _, err := db.ExecContext(ctx, `UPDATE verification_polls SET status='failed', error=$1 WHERE id=$2`, "no_client", p.ID); err != nil {
			slog.Error("mark no_client failed", "error", err, "id", p.ID)
		}
		return
	}

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	gatewayStatus, gatewayAmount, raw, err := client.Status(cctx, p.TxnID)
	if err != nil {
		// record error and schedule next attempt
		if _, err2 := db.ExecContext(ctx, `UPDATE verification_polls SET status='failed', error=$1, completed_at=now() WHERE id=$2`, err.Error(), p.ID); err2 != nil {
			slog.Error("update poll failed status failed", "error", err2, "id", p.ID)
		}
		// schedule next attempt if under attempt cap
		if p.Attempt < 8 {
			nextDelay := NextDelay(p.Attempt+1, lag.ScheduleFor(p.Gateway).CatchPolls, time.Duration(cfg.MaxBackoffS)*time.Second)
			scheduledAt := time.Now().Add(nextDelay)
			if _, err3 := db.ExecContext(ctx, `INSERT INTO verification_polls (txn_id, attempt_number, scheduled_at, status) VALUES ($1, $2, $3, 'pending')`, p.TxnID, p.Attempt+1, scheduledAt); err3 != nil {
				slog.Error("schedule next attempt failed", "error", err3)
			}
		}
		return
	}

	// write successful poll result
	var rawJSON json.RawMessage
	if raw != nil {
		rawJSON = raw
	}
	if _, err := db.ExecContext(ctx, `UPDATE verification_polls SET gateway_status=$1, gateway_amount=$2, raw_response=$3, completed_at=now(), status='completed' WHERE id=$4`, gatewayStatus, gatewayAmount, rawJSON, p.ID); err != nil {
		slog.Error("update poll success failed", "error", err, "id", p.ID)
		return
	}

	// check N-of-N consensus
	ok, err := checkConsensus(ctx, db, p.TxnID, cfg.StabilizationN)
	if err != nil {
		slog.Error("consensus check failed", "error", err, "txn", p.TxnID)
		return
	}
	if ok {
		if err := finalizeHold(ctx, db, p.TxnID); err != nil {
			slog.Error("finalize hold failed", "error", err, "txn", p.TxnID)
		}
		return
	}

	// not stabilized yet — schedule next attempt if allowed
	if p.Attempt < 8 {
		nextDelay := NextDelay(p.Attempt+1, lag.ScheduleFor(p.Gateway).CatchPolls, time.Duration(cfg.MaxBackoffS)*time.Second)
		scheduledAt := time.Now().Add(nextDelay)
		if _, err := db.ExecContext(ctx, `INSERT INTO verification_polls (txn_id, attempt_number, scheduled_at, status) VALUES ($1, $2, $3, 'pending')`, p.TxnID, p.Attempt+1, scheduledAt); err != nil {
			slog.Error("insert next attempt failed", "error", err)
		}
	}
}

func checkConsensus(ctx context.Context, db *sql.DB, txnID string, N int) (bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT gateway_status FROM verification_polls WHERE txn_id=$1 AND gateway_status IS NOT NULL ORDER BY completed_at DESC LIMIT $2`, txnID, N)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var statuses []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return false, err
		}
		if !s.Valid {
			return false, nil
		}
		statuses = append(statuses, s.String)
	}
	if len(statuses) != N {
		return false, nil
	}
	for _, v := range statuses {
		if v != statuses[0] {
			return false, nil
		}
	}
	return true, nil
}

func mapGatewayStatusToHold(status string) string {
	switch status {
	case "success", "captured", "completed":
		return "CONFIRMED"
	case "failed", "failure":
		return "FAILED"
	default:
		return ""
	}
}

func finalizeHold(ctx context.Context, db *sql.DB, txnID string) error {
	// determine latest gateway_status
	row := db.QueryRowContext(ctx, `SELECT gateway_status FROM verification_polls WHERE txn_id=$1 AND gateway_status IS NOT NULL ORDER BY completed_at DESC LIMIT 1`, txnID)
	var latest sql.NullString
	if err := row.Scan(&latest); err != nil {
		return err
	}
	if !latest.Valid {
		return fmt.Errorf("no gateway_status available for %s", txnID)
	}
	mapped := mapGatewayStatusToHold(latest.String)
	if mapped == "" {
		// nothing to do
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var fromStatus sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status FROM holds WHERE txn_id=$1 FOR UPDATE`, txnID).Scan(&fromStatus); err != nil {
		return err
	}
	if fromStatus.Valid && (fromStatus.String == "CONFIRMED" || fromStatus.String == "FAILED" || fromStatus.String == "REFUNDED") {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE holds SET status=$1 WHERE txn_id=$2`, mapped, txnID); err != nil {
		return err
	}
	detail := json.RawMessage(fmt.Sprintf(`{"gateway_status":"%s"}`, latest.String))
	if _, err := tx.ExecContext(ctx, `INSERT INTO ledger (txn_id, event_type, source, from_status, to_status, detail) VALUES ($1, 'state_transition', 'stabilizer', $2, $3, $4)`, txnID, fromStatus.String, mapped, detail); err != nil {
		return err
	}
	payload := json.RawMessage(fmt.Sprintf(`{"txn_id":"%s","status":"%s"}`, txnID, mapped))
	idempotency := "evt_" + txnID + "_" + mapped
	if _, err := tx.ExecContext(ctx, `INSERT INTO outbox (txn_id, event_type, payload, idempotency_key, next_attempt_at) VALUES ($1, $2, $3, $4, now())`, txnID, "transaction."+mapped, payload, idempotency); err != nil {
		return err
	}
	return tx.Commit()
}
