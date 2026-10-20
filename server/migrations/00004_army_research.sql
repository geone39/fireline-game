-- +goose Up
-- Army and lab research (docs/04-data-model.md). Additive only (expand/contract).
-- Queued training/research live in timers (kind = 'train' | 'research'); these tables hold the results.

CREATE TABLE army (
    player_id uuid NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    unit_id   text NOT NULL,
    level     int  NOT NULL CHECK (level >= 1),
    count     int  NOT NULL CHECK (count >= 0),
    PRIMARY KEY (player_id, unit_id, level)
);

-- state: 'researching' (finish_at set) -> 'done' (finish_at may be kept or cleared).
CREATE TABLE research (
    player_id uuid        NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    node_id   text        NOT NULL,
    state     text        NOT NULL CHECK (state IN ('researching', 'done')),
    finish_at timestamptz,
    PRIMARY KEY (player_id, node_id),
    CHECK (state <> 'researching' OR finish_at IS NOT NULL)
);

-- "which trainings/researches does this player have in flight" without scanning all timers of the player.
CREATE INDEX timers_player_kind_idx ON timers (player_id, kind);

-- +goose Down
DROP INDEX timers_player_kind_idx;
DROP TABLE research;
DROP TABLE army;
