
-- +migrate Up

CREATE TYPE auth_service_type AS ENUM ('intr', 'ldap');
CREATE TYPE mfa_auth_type AS ENUM ('none', 'totp', 'cert');
CREATE TYPE inet_range_type AS (start_addr inet, end_addr inet);
CREATE TYPE wg_peer_type AS ENUM ('client', 'server');
CREATE TYPE mfa_cert_type AS ENUM ('ca', 'crl');
-- FIXME use "<=" instead of "<"
CREATE DOMAIN inet_range AS inet_range_type CHECK ((VALUE).start_addr < (VALUE).end_addr);

CREATE TABLE IF NOT EXISTS users (
    id serial PRIMARY KEY,
    username varchar(50) NOT NULL,
    password varchar NOT NULL,
    is_admin boolean NOT NULL,
    auth_service auth_service_type NOT NULL DEFAULT 'intr',
    vpn_type varchar(20),
    mfa_auth mfa_auth_type NOT NULL DEFAULT 'none',
    ntf varchar(75) NOT NULL DEFAULT '',
    updated_at timestamp NOT NULL
);

CREATE TABLE IF NOT EXISTS ldap_templates (
    id serial PRIMARY KEY,
    name varchar(50) NOT NULL UNIQUE,
    filter varchar(99),
    iface_name varchar(32),
    iface_addrs cidr[],
    iface_addr_ranges inet_range[],
    is_admin boolean NOT NULL,
    vpn_type varchar(20),
    mfa_auth mfa_auth_type NOT NULL DEFAULT 'none',
    listen_port text,
    mtu text,
    dns varchar(44)[]
);

CREATE TABLE IF NOT EXISTS ldaps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    priority integer NOT NULL,
    host varchar(40) NOT NULL,
    port integer NOT NULL,
    base_dn varchar(40) NOT NULL,
    bind_dn varchar(40) NOT NULL,
    bind_pw varchar(40) NOT NULL,
    user_list_filter varchar(99) NOT NULL,
    username_attribute varchar(40) NOT NULL,
    uid_attribute varchar(40) NOT NULL,
    use_tls boolean NOT NULL,
    fqdn varchar(255) NOT NULL DEFAULT '',
    ca_cert bytea NOT NULL DEFAULT '',
    template_id integer REFERENCES ldap_templates (id)
);

CREATE TABLE IF NOT EXISTS users_to_ldaps (
    id serial PRIMARY KEY,
    user_id integer REFERENCES users ON DELETE CASCADE,
    ldap_id uuid REFERENCES ldaps ON DELETE CASCADE,
    user_uid varchar(40) NOT NULL,
    user_dn varchar(200) NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL
);

CREATE TABLE IF NOT EXISTS servers (
    id serial PRIMARY KEY,
    name varchar(50) NOT NULL,
    endpoint inet NOT NULL,
    vpn_type varchar(20) NOT NULL,
    description varchar(250) NOT NULL DEFAULT '',
    running_replicas smallint NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS ldap_template_servers (
    id serial PRIMARY KEY,
    ldap_template_id serial REFERENCES ldap_templates (id) NOT NULL,
    server_id integer REFERENCES servers ON DELETE CASCADE,
    allowed_ips cidr[] NOT NULL,
    preshared_key varchar(44)
);

CREATE TABLE IF NOT EXISTS wg_configs (
    id serial PRIMARY KEY,
    user_id integer REFERENCES users ON DELETE CASCADE,
    server_id integer REFERENCES servers ON DELETE CASCADE,
    name varchar(32) NOT NULL,
    private_key varchar(44) NOT NULL,
    public_key varchar(44) NOT NULL,
    listen_port integer,
    mtu integer,
    persistent_keepalive integer
);

CREATE TABLE IF NOT EXISTS wg_peers (
    id serial PRIMARY KEY,
    user_id integer REFERENCES users ON DELETE CASCADE,
    server_id integer REFERENCES servers ON DELETE CASCADE,
    preshared_key varchar(44)
);

-- +migrate StatementBegin

--TODO Currently also triggers for user side updates which leads to unnecessary server updates. However, it might be useful as we haven't yet agreed on how to handle user side updates.
CREATE OR REPLACE FUNCTION notify_wg_peers_update ()
    RETURNS TRIGGER
    AS $$
DECLARE
BEGIN
    PERFORM
        pg_notify('peer_inserted', (
                SELECT
                    json_build_object(
                        'oldServerId', OLD.server_id,
                        'newServerId', NEW.server_id,
                        'allowedIps', json_agg(addr),
                        'presharedKey', NEW.preshared_key,
                        'oldUserPublicKey', (
                            SELECT
                                public_key
                            FROM wg_configs
                            WHERE
                                user_id = OLD.user_id),
                        'newUserPublicKey', (
                            SELECT
                                public_key
                            FROM wg_configs
                            WHERE
                                user_id = NEW.user_id))
                FROM wg_peer_allowed_ip
                WHERE
                    peer_id = NEW.id
                    AND peer = 'client')::text);
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_trigger WHERE tgname = 'trigger_wg_peers_update') THEN
        CREATE CONSTRAINT TRIGGER trigger_wg_peers_update
        AFTER INSERT OR UPDATE OR DELETE ON wg_peers INITIALLY DEFERRED
        FOR EACH ROW EXECUTE PROCEDURE notify_wg_peers_update ();
    END IF;
END
$$;
-- +migrate StatementEnd

CREATE TABLE IF NOT EXISTS wg_iface_addresses (
    id serial PRIMARY KEY,
    config_id integer REFERENCES wg_configs ON DELETE CASCADE,
    addr inet NOT NULL
);

CREATE TABLE IF NOT EXISTS wg_iface_dns (
    id serial PRIMARY KEY,
    config_id integer REFERENCES wg_configs ON DELETE CASCADE,
    dns varchar(44) NOT NULL
);

CREATE TABLE IF NOT EXISTS wg_peer_allowed_ip (
    id serial PRIMARY KEY,
    peer_id integer REFERENCES wg_peers ON DELETE CASCADE,
    addr cidr NOT NULL,
    peer wg_peer_type NOT NULL
);

CREATE TABLE IF NOT EXISTS tags (
    id serial PRIMARY KEY,
    server_id integer REFERENCES servers ON DELETE CASCADE,
    tag varchar(32) NOT NULL DEFAULT '',
    created timestamp NOT NULL
);

CREATE TABLE IF NOT EXISTS root_ca (
    id serial PRIMARY KEY,
    cert bytea NOT NULL DEFAULT '',
    key bytea NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS totp (
    id serial PRIMARY KEY,
    user_id integer REFERENCES users ON DELETE CASCADE,
    key varchar(32) NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS mfa_certs (
    id serial PRIMARY KEY,
    cert bytea NOT NULL DEFAULT '',
    cert_type mfa_cert_type NOT NULL
);

/*
 * Add default admin user.
 */
INSERT INTO users (username, PASSWORD, is_admin, updated_at)
    SELECT 'admin',
        /* Password is 'pwadmin' (hashed using bcrypt) */
        '$2y$04$DK8gh3iNbm1eHpbi.M7./utqHO4eDbb8wytwKgqHh3/b6JajQfG0K', TRUE, '0001-01-01'
    WHERE NOT EXISTS (SELECT * FROM users WHERE username = 'admin');


-- +migrate Down
