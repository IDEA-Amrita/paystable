BEGIN;

-- Note: a cross-time UNIQUE constraint on (gateway, gateway_event_id) is not
-- possible on a table partitioned by received_at, because Postgres requires a
-- unique constraint to include the partition key. Deduplication is therefore
-- handled in the application layer (see internal/webhook/handler.go). We keep a
-- plain lookup index to make that check fast.

CREATE INDEX IF NOT EXISTS idx_webhooks_dedup_lookup
    ON webhooks (gateway, gateway_event_id)
    WHERE gateway_event_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS webhooks_y2026m07 PARTITION OF webhooks
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE IF NOT EXISTS webhooks_y2026m08 PARTITION OF webhooks
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE IF NOT EXISTS webhooks_y2026m09 PARTITION OF webhooks
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE IF NOT EXISTS webhooks_y2026m10 PARTITION OF webhooks
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE IF NOT EXISTS webhooks_y2026m11 PARTITION OF webhooks
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE IF NOT EXISTS webhooks_y2026m12 PARTITION OF webhooks
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

COMMIT;
