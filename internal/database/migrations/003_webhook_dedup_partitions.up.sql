BEGIN;

ALTER TABLE webhooks DROP CONSTRAINT IF EXISTS webhooks_txn_id_fkey;

DO $$
DECLARE
    is_partitioned boolean;
BEGIN
    SELECT c.relkind = 'p'
    INTO is_partitioned
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relname = 'webhooks';

    IF is_partitioned THEN
        CREATE TABLE webhooks_unpartitioned (
            id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
            txn_id           text NOT NULL,
            gateway          text NOT NULL,
            gateway_event_id text,
            event_type       text NOT NULL,
            payload          jsonb NOT NULL,
            received_at      timestamptz NOT NULL DEFAULT now()
        );

        INSERT INTO webhooks_unpartitioned
            (id, txn_id, gateway, gateway_event_id, event_type, payload, received_at)
            OVERRIDING SYSTEM VALUE
        SELECT id, txn_id, gateway, gateway_event_id, event_type, payload, received_at
        FROM webhooks;

        DROP TABLE webhooks CASCADE;
        ALTER TABLE webhooks_unpartitioned RENAME TO webhooks;
        ALTER INDEX webhooks_unpartitioned_pkey RENAME TO webhooks_pkey;

        CREATE INDEX idx_webhooks_txn_id ON webhooks (txn_id);
        CREATE INDEX idx_webhooks_gateway_event_id ON webhooks (gateway, gateway_event_id);
        CREATE INDEX idx_webhooks_received_at ON webhooks (received_at);

        PERFORM setval(
            pg_get_serial_sequence('webhooks', 'id'),
            COALESCE((SELECT max(id) FROM webhooks), 1),
            EXISTS (SELECT 1 FROM webhooks)
        );
    END IF;
END $$;

DELETE FROM webhooks w
USING (
    SELECT ctid,
           row_number() OVER (
               PARTITION BY gateway, gateway_event_id
               ORDER BY received_at, id
           ) AS duplicate_number
    FROM webhooks
    WHERE gateway_event_id IS NOT NULL
) d
WHERE w.ctid = d.ctid
  AND d.duplicate_number > 1;

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
