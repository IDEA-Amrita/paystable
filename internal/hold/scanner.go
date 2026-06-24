package hold

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/IDEA-Amrita/paystable/internal/gateway"
)

// 1)StartExpiryScanner runs a background loop that ticks every 30 seconds
// to scan and process holds that have reached their expiration time.
func StartExpiryScanner(ctx context.Context, db *sql.DB, clientFactory func(string) gateway.GatewayClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	slog.Info("hold expiry scanner started", "interval", "30s")

	for {
		select {
		case <-ctx.Done():
			slog.Info("hold expiry scanner stopping")
			return
		case <-ticker.C: //communication channel for ticker
			scanExpiredHolds(ctx, db, clientFactory)
		}
	}
}

// scanExpiredHolds claims up to 10 expired PENDING holds, transitions them
// to VERIFYING status, and processes them concurrently in a worker pool.
func scanExpiredHolds(ctx context.Context, db *sql.DB, clientFactory func(string) gateway.GatewayClient) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("expiry scanner: begin tx failed", "error", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT txn_id, gateway, amount
		FROM holds
		WHERE (status = 'PENDING' AND expires_at <= now())
		   OR (status = 'VERIFYING' AND updated_at <= now() - interval '5 minutes')
		ORDER BY expires_at
		LIMIT 10
		FOR UPDATE OF holds SKIP LOCKED`)
	if err != nil {
		slog.Error("expiry scanner: query failed", "error", err)
		_ = tx.Rollback()
		return
	}

	type expiredHold struct {
		TxnID   string
		Gateway string
		Amount  int64
	}
	var batch []expiredHold
	for rows.Next() {
		var h expiredHold
		if err := rows.Scan(&h.TxnID, &h.Gateway, &h.Amount); err != nil {
			slog.Error("expiry scanner: scan failed", "error", err)
			continue
		}
		batch = append(batch, h)
	}
	if err := rows.Close(); err != nil {
		slog.Error("expiry scanner: close rows failed", "error", err)
		_ = tx.Rollback()
		return
	}

	if err := rows.Err(); err != nil {
		slog.Error("expiry scanner: rows iteration failed", "error", err)
		_ = tx.Rollback()
		return
	}

	if len(batch) == 0 {
		_ = tx.Rollback()
		return
	}

	for _, h := range batch {
		if _, err := tx.ExecContext(ctx,
			`UPDATE holds SET status='VERIFYING' WHERE txn_id=$1`, h.TxnID,
		); err != nil {
			slog.Error("expiry scanner: claim update failed", "txn_id", h.TxnID, "error", err)
			_ = tx.Rollback()
			return
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("expiry scanner: commit claim tx failed", "error", err)
		return
	}

	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

Loop:
	for _, h := range batch {
		h := h
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break Loop
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }() // release slot when done
			processExpiredHold(ctx, db, clientFactory, h.TxnID, h.Gateway, h.Amount)
		}()
	}

	wg.Wait() // wait for all goroutines to finish before returning
}

// 3)processExpiredHold performs a final status check on a single expired hold
// by querying the external gateway and resolving the hold state accordingly.
func processExpiredHold(ctx context.Context, db *sql.DB, clientFactory func(string) gateway.GatewayClient, txnID, gw string, holdAmount int64) {
	client := clientFactory(gw)
	if client == nil {
		slog.Error("expiry scanner: no client for gateway", "gateway", gw, "txn_id", txnID)
		if err := finalizeAsIndeterminate(ctx, db, txnID, "no_gateway_client", 0, holdAmount); err != nil {
			slog.Error("expiry scanner: finalizeAsIndeterminate failed", "txn_id", txnID, "error", err)
		}
		return
	}

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	gatewayStatus, gatewayAmount, _, err := client.Status(cctx, txnID)
	if err != nil {
		slog.Error("expiry scanner: gateway call failed", "txn_id", txnID, "error", err)
		if err2 := finalizeAsIndeterminate(ctx, db, txnID, "gateway_error", 0, holdAmount); err2 != nil {
			slog.Error("expiry scanner: finalizeAsIndeterminate failed", "txn_id", txnID, "error", err2)
		}
		return
	}

	slog.Info("expiry scanner: gateway responded", "txn_id", txnID, "gateway_status", gatewayStatus, "gateway_amount", gatewayAmount)

	switch {
	case isSuccess(gatewayStatus) && gatewayAmount == holdAmount:
		// Gateway confirms success and the amount matches — CONFIRMED.
		if err := finalizeAsConfirmed(ctx, db, txnID); err != nil {
			slog.Error("expiry scanner: finalizeAsConfirmed failed", "txn_id", txnID, "error", err)
		}
	case isSuccess(gatewayStatus) && gatewayAmount != holdAmount:
		if err := finalizeAsMismatch(ctx, db, txnID, gatewayAmount, holdAmount); err != nil {
			slog.Error("expiry scanner: finalizeAsMismatch failed", "txn_id", txnID, "error", err)
		}
	default:
		reason := "ttl_expired_gateway_status: " + gatewayStatus
		if gatewayStatus == "failed" || gatewayStatus == "failure" {
			if err := finalizeAsFailed(ctx, db, txnID, reason); err != nil {
				slog.Error("expiry scanner: finalizeAsFailed failed", "txn_id", txnID, "error", err)
			}
			return
		}
		if err := finalizeAsIndeterminate(ctx, db, txnID, reason, gatewayAmount, holdAmount); err != nil {
			slog.Error("expiry scanner: finalizeAsIndeterminate failed", "txn_id", txnID, "error", err)
		}
	}
}

// 4) isSuccess returns true if the status received from the external gateway
// represents a final successful payment status (e.g., success, captured).
func isSuccess(s string) bool {
	switch s {
	case "success", "captured", "completed":
		return true
	}
	return false
}

// 5)finalizeAsConfirmed transitions the expired hold to CONFIRMED and enqueues
// a confirmation event in the outbox table for delivery to the client.
func finalizeAsConfirmed(ctx context.Context, db *sql.DB, txnID string) error {
	return finalizeHold(ctx, db, txnID, "CONFIRMED", "ttl_expiry_success", func(tx *sql.Tx) error {
		payloadBytes, err := json.Marshal(struct {
			TxnID  string `json:"txn_id"`
			Status string `json:"status"`
		}{TxnID: txnID, Status: "CONFIRMED"})
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		idempotency := "evt_" + txnID + "_CONFIRMED"
		_, err = tx.ExecContext(ctx,
			`INSERT INTO outbox (txn_id, event_type, payload, idempotency_key, next_attempt_at)
			 VALUES ($1, 'transaction.CONFIRMED', $2, $3, now())
			 ON CONFLICT (idempotency_key) DO NOTHING`,
			txnID, json.RawMessage(payloadBytes), idempotency)
		return err
	})
}

// 6)finalizeAsFailed transitions the expired hold to FAILED and enqueues
// a failure event in the outbox table to notify the client application.
func finalizeAsFailed(ctx context.Context, db *sql.DB, txnID, reason string) error {
	return finalizeHold(ctx, db, txnID, "FAILED", reason, func(tx *sql.Tx) error {
		payloadBytes, err := json.Marshal(struct {
			TxnID  string `json:"txn_id"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		}{TxnID: txnID, Status: "FAILED", Reason: reason})
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		idempotency := "evt_" + txnID + "_FAILED_ttl_expiry"
		_, err = tx.ExecContext(ctx,
			`INSERT INTO outbox (txn_id, event_type, payload, idempotency_key, next_attempt_at)
			 VALUES ($1, 'transaction.FAILED', $2, $3, now())
			 ON CONFLICT (idempotency_key) DO NOTHING`,
			txnID, json.RawMessage(payloadBytes), idempotency)
		return err
	})
}

// 7)finalizeAsIndeterminate transitions the hold to INDETERMINATE when the payment
// amount reported by the gateway does not match the expected hold amount.
func finalizeAsMismatch(ctx context.Context, db *sql.DB, txnID string, gatewayAmount, holdAmount int64) error {
	return finalizeWithReview(ctx, db, txnID, "MISMATCH", "amount_mismatch", gatewayAmount, holdAmount)
}

func finalizeAsIndeterminate(ctx context.Context, db *sql.DB, txnID, reason string, gatewayAmount, holdAmount int64) error {
	return finalizeWithReview(ctx, db, txnID, "INDETERMINATE", reason, gatewayAmount, holdAmount)
}

func finalizeWithReview(ctx context.Context, db *sql.DB, txnID, status, reason string, gatewayAmount, holdAmount int64) error {
	return finalizeHold(ctx, db, txnID, status, reason, func(tx *sql.Tx) error {
		payloadBytes, err := json.Marshal(struct {
			TxnID         string `json:"txn_id"`
			Status        string `json:"status"`
			Reason        string `json:"reason"`
			GatewayAmount int64  `json:"gateway_amount"`
			HoldAmount    int64  `json:"hold_amount"`
		}{TxnID: txnID, Status: status, Reason: reason, GatewayAmount: gatewayAmount, HoldAmount: holdAmount})
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		idempotency := "evt_" + txnID + "_" + status
		_, err = tx.ExecContext(ctx,
			`INSERT INTO outbox (txn_id, event_type, payload, idempotency_key, next_attempt_at)
			 VALUES ($1, $2, $3, $4, now())
			 ON CONFLICT (idempotency_key) DO NOTHING`,
			txnID, "transaction."+status, json.RawMessage(payloadBytes), idempotency)
		return err
	})
}

// 8)finalizeHold is a helper that wraps the database state transitions, ledger logging,
// and event queue writing in a single, safe transaction.
func finalizeHold(ctx context.Context, db *sql.DB, txnID, toStatus, reason string, writeOutbox func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var fromStatus string
	if err := tx.QueryRowContext(ctx,
		`SELECT status FROM holds WHERE txn_id=$1 FOR UPDATE`, txnID,
	).Scan(&fromStatus); err != nil {
		return err
	}

	// Already finalised by another path (stabilizer, manual ops, etc.) — skip.
	switch fromStatus {
	case "CONFIRMED", "FAILED", "REFUNDED", "INDETERMINATE", "MISMATCH":
		slog.Info("expiry scanner: hold already finalised, skipping",
			"txn_id", txnID, "current_status", fromStatus)
		return tx.Commit()
	}

	// Transition the hold status.
	if _, err := tx.ExecContext(ctx,
		`UPDATE holds SET status=$1 WHERE txn_id=$2`, toStatus, txnID,
	); err != nil {
		return err
	}

	// Write an audit ledger entry so ops can see this came from the TTL sweeper.
	detailBytes, err := json.Marshal(struct {
		Reason string `json:"reason"`
	}{Reason: reason})
	if err != nil {
		return fmt.Errorf("marshal ledger detail: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO ledger (txn_id, event_type, source, from_status, to_status, detail)
		 VALUES ($1, 'state_transition', 'ttl_sweeper', $2, $3, $4)`,
		txnID, fromStatus, toStatus, json.RawMessage(detailBytes),
	); err != nil {
		return err
	}

	// Write the outbox event (delivery worker will send merchant callback).
	if err := writeOutbox(tx); err != nil {
		return err
	}

	return tx.Commit()
}
