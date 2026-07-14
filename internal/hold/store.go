package hold

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var ErrCreateConflict = errors.New("hold already exists with different create parameters")

type Hold struct {
	ID          int64     `json:"-"`
	TxnID       string    `json:"txn_id"`
	Gateway     string    `json:"gateway"`
	Status      string    `json:"status"`
	Amount      int64     `json:"amount"`
	Currency    string    `json:"currency"`
	ReadToken   string    `json:"read_token"`
	CallbackURL string    `json:"-"`
	TTLSeconds  int       `json:"-"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a hold and atomically schedules the first verification poll.
// A duplicated txn_id is idempotent only when the create parameters are the
// same as the original request.
func (s *Store) Create(txnID, gateway, callbackURL, currency string, amount int64, ttl int, metadata []byte) (*Hold, error) {
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}

	readToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(ttl) * time.Second)

	// Begin a transaction so the hold row and the initial poll row are written
	// atomically. If either insert fails the whole unit rolls back.
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	h := &Hold{}
	err = tx.QueryRow(`
		INSERT INTO holds (txn_id, gateway, amount, currency, read_token, callback_url, ttl_seconds, expires_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (txn_id) DO NOTHING
		RETURNING id, txn_id, gateway, status, amount, currency, read_token, expires_at, created_at, updated_at`,
		txnID, gateway, amount, currency, readToken, callbackURL, ttl, expiresAt, metadata,
	).Scan(&h.ID, &h.TxnID, &h.Gateway, &h.Status, &h.Amount, &h.Currency, &h.ReadToken, &h.ExpiresAt, &h.CreatedAt, &h.UpdatedAt)

	if err == sql.ErrNoRows {
		// txn_id already exists — roll back immediately (no locks to hold)
		// and return the existing hold if the parameters match.
		_ = tx.Rollback()
		return s.getByTxnIDIfCreateMatches(txnID, gateway, callbackURL, currency, amount, ttl, metadata)
	}
	if err != nil {
		return nil, fmt.Errorf("insert hold: %w", err)
	}

	// Proactively schedule the first verification poll inside the same
	// transaction. Scheduled 10 seconds out so the gateway has time to
	// reflect the payment before we query. ON CONFLICT DO NOTHING is a
	// safety net for any race that already inserted attempt 1.
	_, err = tx.Exec(`
		INSERT INTO verification_polls (txn_id, attempt_number, scheduled_at, status)
		VALUES ($1, 1, now() + interval '10 seconds', 'pending')
		ON CONFLICT (txn_id, attempt_number) DO NOTHING`,
		h.TxnID)
	if err != nil {
		return nil, fmt.Errorf("schedule initial poll: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return h, nil
}

//get by transaction id if create matches
func (s *Store) getByTxnIDIfCreateMatches(txnID, gateway, callbackURL, currency string, amount int64, ttl int, metadata []byte) (*Hold, error) {
	h := &Hold{}
	err := s.db.QueryRow(`
		SELECT id, txn_id, gateway, status, amount, currency, read_token,
		       callback_url, ttl_seconds, expires_at, created_at, updated_at
		FROM holds
		WHERE txn_id = $1
		  AND gateway = $2
		  AND amount = $3
		  AND currency = $4
		  AND callback_url = $5
		  AND ttl_seconds = $6
		  AND metadata = $7::jsonb`,
		txnID, gateway, amount, currency, callbackURL, ttl, metadata,
	).Scan(
		&h.ID, &h.TxnID, &h.Gateway, &h.Status, &h.Amount, &h.Currency, &h.ReadToken,
		&h.CallbackURL, &h.TTLSeconds, &h.ExpiresAt, &h.CreatedAt, &h.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrCreateConflict
	}
	if err != nil {
		return nil, err
	}
	return h, nil
}

//GetByTxnID returns a hold by transaction id
func (s *Store) GetByTxnID(txnID string) (*Hold, error) {
	h := &Hold{}
	err := s.db.QueryRow(`
		SELECT id, txn_id, gateway, status, amount, currency, read_token, expires_at, created_at, updated_at
		FROM holds WHERE txn_id = $1`, txnID,
	).Scan(&h.ID, &h.TxnID, &h.Gateway, &h.Status, &h.Amount, &h.Currency, &h.ReadToken, &h.ExpiresAt, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return h, nil
}

//GetByTxnIDAndToken returns a hold only when the transaction id & read token matches
func (s *Store) GetByTxnIDAndToken(txnID, token string) (*Hold, error) {
	h := &Hold{}
	err := s.db.QueryRow(`
		SELECT id, txn_id, gateway, status, amount, currency, read_token, expires_at, created_at, updated_at
		FROM holds WHERE txn_id = $1 AND read_token = $2`, txnID, token,
	).Scan(&h.ID, &h.TxnID, &h.Gateway, &h.Status, &h.Amount, &h.Currency, &h.ReadToken, &h.ExpiresAt, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return h, nil
}

//generates a random token of 20 bytes and returns it
func generateToken() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pst_rt_" + hex.EncodeToString(b), nil
}
