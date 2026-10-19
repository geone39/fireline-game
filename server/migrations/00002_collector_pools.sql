-- +goose Up
-- Collector pool state (OQ-E1): accrued-but-uncollected resources per player.
-- Forward-compatible: additive only, new table, no change to existing ones.
-- amount is the pool value as of updated_at; svc/economy accrues lazily from there.

CREATE TABLE collector_pools (
    player_id  uuid        NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    resource   text        NOT NULL,
    amount     bigint      NOT NULL DEFAULT 0 CHECK (amount >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, resource)
);

-- +goose Down
DROP TABLE collector_pools;
