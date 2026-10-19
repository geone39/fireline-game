-- +goose Up
-- Immutable defence snapshots (OQ-B7, docs/04-data-model.md). One row per saved layout
-- version; raids attack a snapshot, never the live base. Additive only.

CREATE TABLE defence_snapshots (
    id           uuid        PRIMARY KEY,
    player_id    uuid        NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    base_version int         NOT NULL,
    hq_level     int         NOT NULL,
    layout       jsonb       NOT NULL,
    garrison     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (player_id, base_version)
);

CREATE INDEX defence_snapshots_player_idx ON defence_snapshots (player_id, created_at DESC);

-- +goose StatementBegin
CREATE FUNCTION defence_snapshots_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'defence_snapshots is immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER defence_snapshots_no_update
    BEFORE UPDATE ON defence_snapshots
    FOR EACH ROW EXECUTE FUNCTION defence_snapshots_immutable();

-- +goose Down
DROP TABLE defence_snapshots;
DROP FUNCTION defence_snapshots_immutable();
