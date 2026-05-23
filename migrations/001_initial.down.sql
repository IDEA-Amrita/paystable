BEGIN;

DROP TABLE IF EXISTS gateway_secrets;
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS ledger;
DROP TABLE IF EXISTS verification_polls;
DROP TABLE IF EXISTS webhooks_rejected;
DROP TABLE IF EXISTS webhooks_y2026m06;
DROP TABLE IF EXISTS webhooks_y2026m05;
DROP TABLE IF EXISTS webhooks;
DROP TRIGGER IF EXISTS trg_hold_transitions ON holds;
DROP FUNCTION IF EXISTS enforce_hold_transitions();
DROP TABLE IF EXISTS holds;

COMMIT;
