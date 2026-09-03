
-- +migrate Up
ALTER TABLE users
    ADD session_id VARCHAR(32) NOT NULL DEFAULT '';

ALTER TABLE users
    ADD session_updated_at timestamp;

-- +migrate Down
ALTER TABLE users DROP COLUMN session_id;
ALTER TABLE users DROP COLUMN session_updated_at;