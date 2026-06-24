BEGIN;
--(1)
--1.1)local record for a checkout/transaction
CREATE TABLE holds (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    txn_id      text NOT NULL UNIQUE,
    gateway     text NOT NULL,
    status      text NOT NULL DEFAULT 'PENDING',
    amount      bigint NOT NULL,
    currency    text NOT NULL DEFAULT 'INR',
    read_token  text NOT NULL UNIQUE,
    callback_url text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}',
    ttl_seconds int NOT NULL DEFAULT 300,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_holds_status CHECK (status IN ('PENDING','VERIFYING','CONFIRMED','FAILED','REFUNDED','INDETERMINATE','MISMATCH')),
    CONSTRAINT chk_holds_amount_positive CHECK (amount > 0),
    CONSTRAINT chk_holds_ttl_range CHECK (ttl_seconds BETWEEN 30 AND 900)
);

CREATE INDEX idx_holds_status ON holds (status) WHERE status IN ('PENDING', 'VERIFYING');
CREATE INDEX idx_holds_expires_at ON holds (expires_at) WHERE status = 'PENDING';
CREATE INDEX idx_holds_gateway_status ON holds (gateway, status);

--1.2)state transition enforcement trigger(prevents illegal transitions)
CREATE OR REPLACE FUNCTION enforce_hold_transitions() RETURNS trigger AS $$
BEGIN
    IF OLD.status IN ('CONFIRMED', 'FAILED', 'REFUNDED', 'INDETERMINATE', 'MISMATCH') THEN
        IF NOT (OLD.status = 'CONFIRMED' AND NEW.status = 'REFUNDED') THEN
            RAISE EXCEPTION 'illegal transition from % to %', OLD.status, NEW.status;
        END IF;
    END IF;
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_hold_transitions
    BEFORE UPDATE ON holds
    FOR EACH ROW
    EXECUTE FUNCTION enforce_hold_transitions();

--(2)
--2.1)webhooks: standard unpartitioned table
CREATE TABLE webhooks (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    txn_id          text NOT NULL,
    gateway         text NOT NULL,
    gateway_event_id text,
    event_type      text NOT NULL,
    payload         jsonb NOT NULL,
    received_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhooks_txn_id ON webhooks (txn_id);
CREATE INDEX idx_webhooks_gateway_event_id ON webhooks (gateway, gateway_event_id);
CREATE INDEX idx_webhooks_received_at ON webhooks (received_at);

--2.3)webhooks_rejected: quarantine for failed HMAC
CREATE TABLE webhooks_rejected (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    gateway         text NOT NULL,
    rejection_reason text NOT NULL,
    headers         jsonb NOT NULL,
    raw_body        bytea NOT NULL,
    source_ip       inet,
    received_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_rejected_received_at ON webhooks_rejected (received_at);
CREATE INDEX idx_rejected_gateway ON webhooks_rejected (gateway);

--3)verification_polls: implements retries, and provides a full timestamped history of every check for debugging/audit.
CREATE TABLE verification_polls (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    txn_id          text NOT NULL REFERENCES holds(txn_id),
    attempt_number  int NOT NULL,
    status          text NOT NULL DEFAULT 'pending',
    gateway_status  text,
    gateway_amount  bigint,
    raw_response    jsonb,
    scheduled_at    timestamptz NOT NULL,
    started_at      timestamptz,
    completed_at    timestamptz,
    error           text,

    CONSTRAINT chk_poll_status CHECK (status IN ('pending','in_flight','completed','failed'))
);

CREATE INDEX idx_polls_job_queue ON verification_polls (scheduled_at) WHERE status = 'pending';
CREATE INDEX idx_polls_txn_id ON verification_polls (txn_id);

--4)ledger: usiness audit trail of decisions/state transitions
CREATE TABLE ledger (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    txn_id      text NOT NULL REFERENCES holds(txn_id),
    event_type  text NOT NULL,
    source      text NOT NULL,
    from_status text,
    to_status   text,
    detail      jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ledger_txn_id_created ON ledger (txn_id, created_at);
CREATE INDEX idx_ledger_event_type ON ledger (event_type) WHERE event_type = 'state_transition';

--5)outbox: delivery queue to merchant app
CREATE TABLE outbox (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    txn_id          text NOT NULL REFERENCES holds(txn_id),
    event_type      text NOT NULL,
    payload         jsonb NOT NULL,
    idempotency_key text NOT NULL UNIQUE,
    status          text NOT NULL DEFAULT 'pending',
    attempts        int NOT NULL DEFAULT 0,
    max_attempts    int NOT NULL DEFAULT 8,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_attempt_at timestamptz,
    last_http_status int,
    last_error      text,
    delivered_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_outbox_status CHECK (status IN ('pending','in_flight','delivered','exhausted'))
);

CREATE INDEX idx_outbox_job_queue ON outbox (next_attempt_at) WHERE status = 'pending';
CREATE INDEX idx_outbox_txn_id ON outbox (txn_id);
CREATE INDEX idx_outbox_status ON outbox (status) WHERE status = 'exhausted';

--6)gateway_secrets: stores encrypted webhook signing secrets&webhook signing keys with rotation
CREATE TABLE gateway_secrets (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    gateway             text NOT NULL,
    secret_encrypted    bytea NOT NULL,
    is_active           boolean NOT NULL DEFAULT true,
    rotation_window_end timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    deactivated_at      timestamptz
);

CREATE INDEX idx_secrets_gateway_active ON gateway_secrets (gateway) WHERE is_active = true;

COMMIT;
