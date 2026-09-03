
-- +migrate Up
ALTER TABLE users_to_ldaps
DROP CONSTRAINT users_to_ldaps_ldap_id_fkey,
ADD CONSTRAINT users_to_ldaps_ldap_id_fkey
    FOREIGN KEY (ldap_id)
    REFERENCES ldaps ON DELETE SET NULL;

ALTER TABLE ldap_templates
    ALTER COLUMN iface_name SET DEFAULT '',
    ALTER COLUMN vpn_type SET DEFAULT '',
    ALTER COLUMN listen_port SET DEFAULT '',
    ALTER COLUMN mtu SET DEFAULT '';

UPDATE ldap_templates
SET iface_name = COALESCE(iface_name, ''),
    vpn_type = COALESCE(vpn_type, ''),
    listen_port = COALESCE(listen_port, ''),
    mtu= COALESCE(mtu, '');

-- +migrate Down
ALTER TABLE users_to_ldaps
DROP CONSTRAINT users_to_ldaps_ldap_id_fkey,
ADD CONSTRAINT users_to_ldaps_ldap_id_fkey
    FOREIGN KEY (ldap_id)
    REFERENCES ldaps ON DELETE CASCADE;

ALTER TABLE ldap_templates
    ALTER COLUMN iface_name DROP DEFAULT,
    ALTER COLUMN vpn_type DROP DEFAULT,
    ALTER COLUMN listen_port DROP DEFAULT,
    ALTER COLUMN mtu DROP DEFAULT;

UPDATE ldap_templates
SET iface_name = NULLIF(iface_name, ''),
    vpn_type = NULLIF(vpn_type, ''),
    listen_port = NULLIF(listen_port, ''),
    mtu= NULLIF(mtu, '');