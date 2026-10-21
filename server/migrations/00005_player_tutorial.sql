-- +goose Up
-- One-shot tutorial/provisioning marks of a player (SERVER-5): e.g. 'free_finish_used' (the free
-- finish of the first army order, OQ-ARMY-8), 'start_army_granted'. Additive only (expand/contract).
ALTER TABLE players ADD COLUMN tutorial jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE players DROP COLUMN tutorial;
