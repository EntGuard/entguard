-- +migrate Up
ALTER TABLE users DROP COLUMN IF EXISTS vpn_type;
ALTER TABLE ldap_templates DROP COLUMN IF EXISTS vpn_type;
ALTER TABLE servers DROP COLUMN IF EXISTS vpn_type;

-- +migrate Down
ALTER TABLE users ADD COLUMN vpn_type varchar(20);
ALTER TABLE ldap_templates ADD COLUMN vpn_type varchar(20) DEFAULT '';
ALTER TABLE servers ADD COLUMN vpn_type varchar(20) NOT NULL DEFAULT 'WIREGUARD';
ALTER TABLE servers ALTER COLUMN vpn_type DROP DEFAULT;
