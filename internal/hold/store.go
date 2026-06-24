package hold

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

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

// 1)Create: inserts new hold record, generates read token, returns hold details. If txn_id already exists, returns existing hold.
func (s *Store) Create(txnID, gateway, callbackURL, currency string, amount int64, ttl int, metadata []byte) (*Hold, error) {
	readToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(ttl) * time.Second)

	h := &Hold{}
	err = s.db.QueryRow(`
		INSERT INTO holds (txn_id, gateway, amount, currency, read_token, callback_url, ttl_seconds, expires_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (txn_id) DO NOTHING
		RETURNING id, txn_id, gateway, status, amount, currency, read_token, expires_at, created_at, updated_at`,
		txnID, gateway, amount, currency, readToken, callbackURL, ttl, expiresAt, metadata,
	).Scan(&h.ID, &h.TxnID, &h.Gateway, &h.Status, &h.Amount, &h.Currency, &h.ReadToken, &h.ExpiresAt, &h.CreatedAt, &h.UpdatedAt)

	if err == sql.ErrNoRows {
		//conflict: txn_id already exists, return existing
		return s.GetByTxnID(txnID)
	}
	if err != nil {
		return nil, fmt.Errorf("insert hold: %w", err)
	}

	return h, nil
}

// 2)GetByTxnID: retrieves hold by txn_id(called when there's a conflict on create to return existing hold)
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

// 3)GetByTxnIDAndToken: retrieves hold by txn_id and read token (used for status endpoint to validate read access)
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

func generateToken() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pst_rt_" + hex.EncodeToString(b), nil
}
