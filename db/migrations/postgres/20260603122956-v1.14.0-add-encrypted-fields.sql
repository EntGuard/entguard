
-- +migrate Up

ALTER TABLE devices
    ADD COLUMN private_key_encrypted bytea NOT NULL DEFAULT '',
    ALTER COLUMN private_key DROP NOT NULL;
ALTER TABLE server_wg_configs
    ADD COLUMN private_key_encrypted bytea NOT NULL DEFAULT '',
    ALTER COLUMN private_key DROP NOT NULL;
ALTER TABLE root_ca
    ADD COLUMN key_encrypted bytea NOT NULL DEFAULT '',
    ALTER COLUMN key DROP NOT NULL;
ALTER TABLE totp
    ADD COLUMN key_encrypted bytea NOT NULL DEFAULT '',
    ALTER COLUMN key DROP NOT NULL;
ALTER TABLE ldaps
    ADD COLUMN bind_pw_encrypted bytea NOT NULL DEFAULT '',
    ALTER COLUMN bind_pw DROP NOT NULL;
ALTER TABLE adjacencies
    ADD COLUMN preshared_key_encrypted bytea NOT NULL DEFAULT '';

-- +migrate Down

DELETE FROM devices WHERE private_key_encrypted != '';
DELETE FROM server_wg_configs WHERE private_key_encrypted != '';
DELETE FROM root_ca WHERE key_encrypted != '';
DELETE FROM totp WHERE key_encrypted != '';
DELETE FROM ldaps WHERE bind_pw_encrypted != '';
DELETE FROM adjacencies WHERE preshared_key_encrypted != '';

DELETE FROM devices WHERE private_key IS NULL;
DELETE FROM server_wg_configs WHERE private_key IS NULL;
DELETE FROM root_ca WHERE key IS NULL;
DELETE FROM totp WHERE key IS NULL;
DELETE FROM ldaps WHERE bind_pw IS NULL;
-- Do not "DELETE FROM adjacencies WHERE preshared_key IS NULL"


ALTER TABLE devices
    DROP COLUMN private_key_encrypted,
    ALTER COLUMN private_key SET NOT NULL;
ALTER TABLE server_wg_configs
    DROP COLUMN private_key_encrypted,
    ALTER COLUMN private_key SET NOT NULL;
ALTER TABLE root_ca
    DROP COLUMN key_encrypted,
    ALTER COLUMN key SET NOT NULL;
ALTER TABLE totp
    DROP COLUMN key_encrypted,
    ALTER COLUMN key SET NOT NULL;
ALTER TABLE ldaps
    DROP COLUMN bind_pw_encrypted,
    ALTER COLUMN bind_pw SET NOT NULL;
ALTER TABLE adjacencies
    DROP COLUMN preshared_key_encrypted;
