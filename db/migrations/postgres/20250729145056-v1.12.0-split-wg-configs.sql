-- +migrate Up

-- Split wg_configs
CREATE TABLE user_wg_configs (
    id serial PRIMARY KEY,
    old_user_config_id integer,
    user_id integer REFERENCES users ON DELETE CASCADE NOT NULL,
    interface_name varchar(256) NOT NULL,
    private_key varchar(44) NOT NULL,
    public_key varchar(44) NOT NULL,
    listen_port integer,
    mtu integer
);

CREATE TABLE server_wg_configs (
    id serial PRIMARY KEY,
    old_server_config_id integer,
    server_id integer REFERENCES servers ON DELETE CASCADE NOT NULL,
    interface_name varchar(256) NOT NULL,
    private_key varchar(44) NOT NULL,
    public_key varchar(44) NOT NULL,
    listen_port integer,
    mtu integer,
    persistent_keepalive integer
);

INSERT INTO user_wg_configs (old_user_config_id , user_id, interface_name, private_key, public_key, listen_port, mtu)
    SELECT id, user_id, name, private_key, public_key, listen_port, mtu
    FROM wg_configs
    WHERE user_id IS NOT NULL
        AND server_id IS NULL;

INSERT INTO server_wg_configs (old_server_config_id , server_id, interface_name, private_key, public_key, listen_port, mtu, persistent_keepalive)
    SELECT id, server_id, name, private_key, public_key, listen_port, mtu, persistent_keepalive
    FROM wg_configs
    WHERE user_id IS NULL
        AND server_id IS NOT NULL;

-- Split wg_iface_addresses
CREATE TABLE user_wg_iface_addresses (
    id serial PRIMARY KEY,
    user_config_id integer REFERENCES user_wg_configs ON DELETE CASCADE NOT NULL,
    addr inet NOT NULL
);

CREATE TABLE server_wg_iface_addresses (
    id serial PRIMARY KEY,
    server_config_id integer REFERENCES server_wg_configs ON DELETE CASCADE NOT NULL,
    addr inet NOT NULL
);

INSERT INTO user_wg_iface_addresses (user_config_id, addr)
    SELECT u.id, a.addr
    FROM wg_iface_addresses a
    JOIN user_wg_configs u ON a.config_id = u.old_user_config_id;

INSERT INTO server_wg_iface_addresses (server_config_id, addr)
    SELECT s.id, a.addr
    FROM wg_iface_addresses a
    JOIN server_wg_configs s ON a.config_id = s.old_server_config_id;

DROP TABLE wg_iface_addresses;

-- Split wg_iface_dns
CREATE TABLE user_wg_iface_dnss (
    id serial PRIMARY KEY,
    user_config_id integer REFERENCES user_wg_configs ON DELETE CASCADE NOT NULL,
    dns varchar(256) NOT NULL
);

CREATE TABLE server_wg_iface_dnss (
    id serial PRIMARY KEY,
    server_config_id integer REFERENCES server_wg_configs ON DELETE CASCADE NOT NULL,
    dns varchar(256) NOT NULL
);

INSERT INTO user_wg_iface_dnss (user_config_id, dns)
    SELECT u.id, d.dns
    FROM wg_iface_dns d
    JOIN user_wg_configs u ON d.config_id = u.old_user_config_id;

INSERT INTO server_wg_iface_dnss (server_config_id, dns)
    SELECT s.id, d.dns
    FROM wg_iface_dns d
    JOIN server_wg_configs s ON d.config_id = s.old_server_config_id;

DROP TABLE wg_iface_dns;
DROP TRIGGER trigger_wg_configs_update ON wg_configs restrict;
DROP FUNCTION notify_wg_configs_update restrict;
DROP TABLE wg_configs;
DROP TRIGGER trigger_wg_peers_update ON wg_peers;
DROP FUNCTION notify_wg_peers_update();
ALTER TABLE server_wg_configs DROP COLUMN old_server_config_id;
ALTER TABLE user_wg_configs DROP COLUMN old_user_config_id;

-- +migrate StatementBegin
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
                            FROM user_wg_configs
                            WHERE
                                user_id = OLD.user_id),
                        'newUserPublicKey', (
                            SELECT
                                public_key
                            FROM user_wg_configs
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
    CREATE CONSTRAINT TRIGGER trigger_wg_peers_update
    AFTER INSERT OR UPDATE OR DELETE ON wg_peers INITIALLY DEFERRED
    FOR EACH ROW EXECUTE PROCEDURE notify_wg_peers_update ();
END$$;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION notify_server_wg_configs_update ()
    RETURNS TRIGGER
    AS $$
DECLARE
BEGIN
    PERFORM pg_notify('server_wg_configs_updated', (json_build_object('serverID', NEW.server_id))::text);
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

DO $$
BEGIN
    CREATE CONSTRAINT TRIGGER trigger_server_wg_configs_update
    AFTER UPDATE ON server_wg_configs INITIALLY DEFERRED
    FOR EACH ROW
    WHEN (NEW.server_id is not null)
    EXECUTE PROCEDURE notify_server_wg_configs_update ();
END
$$;
-- +migrate StatementEnd

-- +migrate Down

CREATE TABLE wg_configs (
    id serial PRIMARY KEY,
    user_id integer,
    server_id integer,
    name varchar(32) NOT NULL,
    private_key varchar(44) NOT NULL,
    public_key varchar(44) NOT NULL,
    listen_port integer,
    mtu integer,
    persistent_keepalive integer
);

INSERT INTO wg_configs (user_id, server_id, name, private_key, public_key, listen_port, mtu, persistent_keepalive)
    SELECT user_id, NULL, interface_name, private_key, public_key, listen_port, mtu, NULL FROM user_wg_configs;
INSERT INTO wg_configs (user_id, server_id, name, private_key, public_key, listen_port, mtu, persistent_keepalive)
    SELECT NULL, server_id, interface_name, private_key, public_key, listen_port, mtu, persistent_keepalive FROM server_wg_configs;

CREATE TABLE wg_iface_addresses (
    id serial PRIMARY KEY,
    config_id integer REFERENCES wg_configs ON DELETE CASCADE,
    addr inet NOT NULL
);

INSERT INTO wg_iface_addresses (config_id, addr)
    SELECT user_config_id, addr FROM user_wg_iface_addresses;
INSERT INTO wg_iface_addresses (config_id, addr)
    SELECT server_config_id, addr FROM server_wg_iface_addresses;

CREATE TABLE wg_iface_dns (
    id serial PRIMARY KEY,
    config_id integer REFERENCES wg_configs ON DELETE CASCADE,
    dns varchar(44) NOT NULL
);

INSERT INTO wg_iface_dns (config_id, dns)
    SELECT user_config_id, dns FROM user_wg_iface_dnss;
INSERT INTO wg_iface_dns (config_id, dns)
    SELECT server_config_id, dns FROM server_wg_iface_dnss;

ALTER TABLE wg_configs ADD CONSTRAINT wg_configs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users ON DELETE CASCADE;
ALTER TABLE wg_configs ADD CONSTRAINT wg_configs_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers ON DELETE CASCADE;

DROP TABLE user_wg_iface_addresses;
DROP TABLE server_wg_iface_addresses;
DROP TABLE user_wg_iface_dnss;
DROP TABLE server_wg_iface_dnss;
DROP TABLE user_wg_configs;
DROP TRIGGER trigger_server_wg_configs_update ON server_wg_configs restrict;
DROP FUNCTION notify_server_wg_configs_update restrict;
DROP TABLE server_wg_configs;
DROP TRIGGER trigger_wg_peers_update ON wg_peers;
DROP FUNCTION notify_wg_peers_update();

-- +migrate StatementBegin
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
    CREATE CONSTRAINT TRIGGER trigger_wg_peers_update
    AFTER INSERT OR UPDATE OR DELETE ON wg_peers INITIALLY DEFERRED
    FOR EACH ROW EXECUTE PROCEDURE notify_wg_peers_update ();
END$$;

-- +migrate StatementEnd

-- +migrate StatementBegin
--Note: pg_notify is wrap for NOTIFY that triggers 'wg_configs_updated' channel listeners after transaction end (-> wg_iface_addresses data also written)
CREATE OR REPLACE FUNCTION notify_wg_configs_update ()
    RETURNS TRIGGER
    AS $$
DECLARE
BEGIN
    PERFORM pg_notify('wg_configs_updated', (json_build_object('serverID', NEW.server_id))::text);
RETURN NEW;
END;
$$
LANGUAGE plpgsql;

DO $$
BEGIN
    CREATE CONSTRAINT TRIGGER trigger_wg_configs_update
    AFTER UPDATE ON wg_configs INITIALLY DEFERRED
    FOR EACH ROW
    WHEN (NEW.server_id is not null)
    EXECUTE PROCEDURE notify_wg_configs_update ();
END
$$;
-- +migrate StatementEnd