package stabilizer

import (
	"context"
	"database/sql"
	"time"
)
func AcquireToken(ctx context.Context, db *sql.DB, gateway string, capacity int, refillPerSec float64) (bool, time.Time, error) {
	//1) begin n rollback Transaction if at all error occurs
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, time.Time{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	//2) Read the row with FOR UPDATE to lock it
	var tokens float64
	var lastRefill time.Time
	err = tx.QueryRowContext(ctx, "SELECT tokens, last_refill FROM rate_limits WHERE gateway=$1 FOR UPDATE", gateway).Scan(&tokens, &lastRefill)//FOR UPDATE provides the LOCK
	if err == sql.ErrNoRows {
		now := time.Now().UTC()
		// consume one token from full capacity
		tokensAfter := float64(capacity - 1)//to create a new row if it doesnt exist n henceby consuming 1 token
		_, err = tx.ExecContext(ctx, "INSERT INTO rate_limits (gateway, tokens, last_refill) VALUES ($1, $2, $3)", gateway, tokensAfter, now)
		if err != nil {
			return false, time.Time{}, err
		}
		if err := tx.Commit(); err != nil {
			return false, time.Time{}, err
		}
		return true, time.Time{}, nil
	} else if err != nil {
		return false, time.Time{}, err
	}
   //3)Refill calculations and token consumption
   //A) Computing how many tokens have accumulated
	now := time.Now().UTC()
	elapsed := now.Sub(lastRefill).Seconds()
	accrued := elapsed * refillPerSec//total number of tokens earned in interval
	tokens = tokens + accrued
	capf := float64(capacity)
	if tokens > capf {
		tokens = capf//cap out the tokens at the bucket size
	}
    //B) Consuming a token when one is available
	if tokens >= 1.0 {
		tokens = tokens - 1.0
		_, err = tx.ExecContext(ctx, "UPDATE rate_limits SET tokens=$1, last_refill=$2 WHERE gateway=$3", tokens, now, gateway)
		if err != nil {
			return false, time.Time{}, err
		}
		if err := tx.Commit(); err != nil {
			return false, time.Time{}, err
		}
		return true, time.Time{}, nil
	}
	//C)When no token is available – compute the exact wait time
	needed := 1.0 - tokens
	waitSecs := needed / refillPerSec
	// 1.3 fix: release the FOR UPDATE lock immediately — nothing was written, no need to hold it.
	_ = tx.Rollback()
	// 1.4 fix: anchor nextAvailable to UTC so it is consistent with DB now().
	// Anchor from now, not lastRefill — lastRefill is in the past so lastRefill+wait would already be elapsed.
	nextAvailable := time.Now().UTC().Add(time.Duration(waitSecs * float64(time.Second)))
	return false, nextAvailable, nil
}
