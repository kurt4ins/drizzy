-- +goose Up

CREATE TYPE interaction_action AS ENUM ('like', 'skip');

ALTER TABLE interactions DROP CONSTRAINT IF EXISTS interactions_action_type_check;

ALTER TABLE interactions
    ALTER COLUMN action_type TYPE interaction_action
    USING action_type::interaction_action;

-- +goose Down

ALTER TABLE interactions
    ALTER COLUMN action_type TYPE TEXT
    USING action_type::text;

ALTER TABLE interactions
    ADD CONSTRAINT interactions_action_type_check
    CHECK (action_type IN ('like', 'skip'));

DROP TYPE IF EXISTS interaction_action;
