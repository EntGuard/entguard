
-- +migrate Up

ALTER TABLE totp
    ADD COLUMN last_used_counter bigint NOT NULL DEFAULT 0;

-- +migrate Down

ALTER TABLE totp
    DROP COLUMN last_used_counter;
