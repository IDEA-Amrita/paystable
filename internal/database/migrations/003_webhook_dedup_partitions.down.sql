BEGIN;

ALTER TABLE webhooks DROP CONSTRAINT IF EXISTS webhooks_gateway_event_id_unique;

COMMIT;
