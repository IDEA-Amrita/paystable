BEGIN;

CREATE TABLE IF NOT EXISTS rate_limits (
  gateway text PRIMARY KEY,
  tokens double precision NOT NULL,
  last_refill timestamptz NOT NULL
);

COMMIT;
