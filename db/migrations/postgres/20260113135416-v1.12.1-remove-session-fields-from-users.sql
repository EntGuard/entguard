
-- +migrate Up

-- Remove session_id and session_updated_at columns from users table
-- These fields are now only used in the devices table
ALTER TABLE users DROP COLUMN IF EXISTS session_id;
ALTER TABLE users DROP COLUMN IF EXISTS session_updated_at;

-- +migrate Down

-- Restore session_id and session_updated_at columns to users table
ALTER TABLE users
    ADD session_id VARCHAR(32) NOT NULL DEFAULT '';

ALTER TABLE users
    ADD session_updated_at timestamp;
