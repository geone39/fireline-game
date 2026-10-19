-- +goose Up
-- Core tables from docs/04-data-model.md plus service tables (refresh_tokens, idempotency_keys).

CREATE TABLE players (
    id            uuid PRIMARY KEY,
    created_at    timestamptz NOT NULL DEFAULT now(),
    auth_provider text        NOT NULL,
    ext_id        text        NOT NULL,
    nickname      text        NOT NULL,
    hq_level      int         NOT NULL DEFAULT 1,
    trophies      int         NOT NULL DEFAULT 0,
    league        text        NOT NULL DEFAULT '',
    shield_until  timestamptz,
    shielded      bool        NOT NULL DEFAULT false, -- maintained by a job: a partial index cannot use now()
    last_seen     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (auth_provider, ext_id)
);
CREATE INDEX players_trophies_unshielded_idx ON players (trophies) WHERE NOT shielded;

CREATE TABLE wallets (
    player_id uuid   NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    resource  text   NOT NULL,
    amount    bigint NOT NULL CHECK (amount >= 0),
    PRIMARY KEY (player_id, resource)
);

-- Append-only: wallets must equal SUM(ledger) (nightly reconciliation).
CREATE TABLE ledger (
    id         bigserial   PRIMARY KEY,
    player_id  uuid        NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    resource   text        NOT NULL,
    delta      bigint      NOT NULL,
    reason     text        NOT NULL,
    ref        text,
    idem_key   text UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ledger_player_idx ON ledger (player_id, id);

-- +goose StatementBegin
CREATE FUNCTION ledger_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER ledger_no_update
    BEFORE UPDATE ON ledger
    FOR EACH ROW EXECUTE FUNCTION ledger_append_only();
CREATE TRIGGER ledger_no_delete
    BEFORE DELETE ON ledger
    FOR EACH ROW WHEN (current_setting('fireline.allow_ledger_delete', true) IS DISTINCT FROM 'on')
    EXECUTE FUNCTION ledger_append_only();

CREATE TABLE base_state (
    player_id  uuid PRIMARY KEY REFERENCES players (id) ON DELETE CASCADE,
    version    int         NOT NULL DEFAULT 1,
    layout     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE base_events (
    id         bigserial   PRIMARY KEY,
    player_id  uuid        NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    op         text        NOT NULL,
    args       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    applied_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX base_events_player_idx ON base_events (player_id, id);

CREATE TABLE timers (
    id        uuid PRIMARY KEY,
    player_id uuid        NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    kind      text        NOT NULL,
    target_id text        NOT NULL,
    finish_at timestamptz NOT NULL,
    payload   jsonb       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX timers_finish_at_idx ON timers (finish_at);
CREATE INDEX timers_player_idx ON timers (player_id);

CREATE TABLE refresh_tokens (
    jti        uuid PRIMARY KEY,
    player_id  uuid        NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_player_idx ON refresh_tokens (player_id);

CREATE TABLE idempotency_keys (
    player_id    uuid        NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    key          text        NOT NULL,
    request_hash bytea       NOT NULL,
    status       int,                 -- NULL while the request is in flight
    content_type text,
    response     bytea,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, key)
);
CREATE INDEX idempotency_keys_created_idx ON idempotency_keys (created_at);

-- +goose Down
DROP TABLE idempotency_keys;
DROP TABLE refresh_tokens;
DROP TABLE timers;
DROP TABLE base_events;
DROP TABLE base_state;
DROP TABLE ledger;
DROP FUNCTION ledger_append_only();
DROP TABLE wallets;
DROP TABLE players;
