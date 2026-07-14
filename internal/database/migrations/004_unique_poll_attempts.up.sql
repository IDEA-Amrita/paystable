BEGIN;
CREATE UNIQUE INDEX uq_polls_txn_attempt ON verification_polls (txn_id, attempt_number);
COMMIT;
