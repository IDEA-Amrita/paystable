BEGIN;

ALTER TABLE webhooks DROP CONSTRAINT IF EXISTS webhooks_txn_id_fkey;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'webhooks_gateway_event_id_unique'
    ) THEN
        ALTER TABLE webhooks
            ADD CONSTRAINT webhooks_gateway_event_id_unique
            UNIQUE (gateway, gateway_event_id);
    END IF;
END $$;

COMMIT;
