-- +goose Up

ALTER TABLE profile_photos
    ADD COLUMN telegram_file_id TEXT;

-- +goose Down

ALTER TABLE profile_photos
    DROP COLUMN telegram_file_id;
