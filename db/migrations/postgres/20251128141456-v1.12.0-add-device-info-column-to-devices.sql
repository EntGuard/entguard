-- +migrate Up
ALTER TABLE devices ADD COLUMN device_information text NULL;

-- +migrate Down
ALTER TABLE devices DROP COLUMN device_information;