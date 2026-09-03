
-- +migrate Up
ALTER TABLE servers
    ADD healthcheck_address inet;

-- +migrate Down
ALTER TABLE servers 
    DROP COLUMN healthcheck_address;
