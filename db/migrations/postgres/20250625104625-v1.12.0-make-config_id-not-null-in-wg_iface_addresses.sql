-- +migrate Up

ALTER TABLE tags
    ALTER COLUMN created SET DEFAULT now();

ALTER TABLE users
    ALTER COLUMN updated_at SET DEFAULT now();

ALTER TABLE users_to_ldaps
    ALTER COLUMN updated_at SET DEFAULT now();

ALTER TABLE users_to_ldaps
    ALTER COLUMN created_at SET DEFAULT now();

UPDATE wg_iface_addresses
SET config_id = 0
WHERE config_id IS NULL;

ALTER TABLE wg_iface_addresses
    ALTER COLUMN config_id SET NOT NULL;

UPDATE wg_iface_dns
SET config_id = 0
WHERE config_id IS NULL;

ALTER TABLE wg_iface_dns
    ALTER COLUMN config_id SET NOT NULL;

-- +migrate Down
ALTER TABLE tags
    ALTER COLUMN created SET DEFAULT NULL;

ALTER TABLE users
    ALTER COLUMN updated_at SET DEFAULT NULL;

ALTER TABLE users_to_ldaps
    ALTER COLUMN updated_at SET DEFAULT NULL;

ALTER TABLE users_to_ldaps
    ALTER COLUMN created_at SET DEFAULT NULL;

ALTER TABLE wg_iface_addresses
    ALTER COLUMN config_id DROP NOT NULL;

ALTER TABLE wg_iface_dns
    ALTER COLUMN config_id DROP NOT NULL;
