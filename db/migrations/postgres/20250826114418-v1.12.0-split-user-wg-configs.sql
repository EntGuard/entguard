
-- +migrate Up

CREATE TABLE device_templates (
    id serial PRIMARY KEY,
    user_id integer REFERENCES users ON DELETE CASCADE NOT NULL,
    interface_name varchar(256) NOT NULL,
    listen_port integer,
    mtu integer,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id serial PRIMARY KEY,
    description text NOT NULL DEFAULT '',
    device_template_id integer REFERENCES device_templates ON DELETE CASCADE NOT NULL,
    private_key varchar(44) NOT NULL,
    public_key varchar(44) NOT NULL,
    external_device_id varchar(21) NOT NULL DEFAULT '',
    session_id varchar(32) NOT NULL DEFAULT '',
    last_time_connected timestamp,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE device_template_address_pools (
    id serial PRIMARY KEY,
    device_template_id integer REFERENCES device_templates ON DELETE CASCADE NOT NULL,
    address_pool_id integer REFERENCES address_pools ON DELETE RESTRICT NOT NULL
);

CREATE TABLE device_wg_iface_addresses (
    id serial PRIMARY KEY,
    device_id integer REFERENCES devices ON DELETE CASCADE NOT NULL,
    addr inet NOT NULL
);

CREATE TABLE device_template_wg_iface_dnss (
    id serial PRIMARY KEY,
    device_template_id integer REFERENCES device_templates ON DELETE CASCADE NOT NULL,
    dns varchar(256) NOT NULL
);

CREATE SEQUENCE ap;

INSERT INTO address_pools (name, description, start_addr, end_addr, net_mask)
    SELECT
        'auto-generated-' || nextval('ap'),
        'automatically generated during database migration',
        -- masks of `start_addr` and `end_addr` are ignored, they are overriden with the field `net_mask`
        host(network(addr))::inet,
        host(broadcast(addr))::inet,
        masklen(addr)
    FROM user_wg_iface_addresses;

DELETE FROM address_pools WHERE id IN (
    SELECT b.id FROM address_pools a
        JOIN address_pools b USING (start_addr, end_addr, net_mask)
        WHERE a.id < b.id
);

DROP SEQUENCE ap;

INSERT INTO device_templates (id, user_id, interface_name, listen_port, mtu)
    SELECT id, user_id, interface_name, listen_port, mtu FROM user_wg_configs;

-- `device` and `device_template` have the same id for now, it simplifies migration
INSERT INTO devices (id, device_template_id, private_key, public_key)
    SELECT wgc.id, wgc.id, wgc.private_key, wgc.public_key
    FROM user_wg_configs wgc
    JOIN users u ON wgc.user_id = u.id;

INSERT INTO device_template_address_pools (id, device_template_id, address_pool_id)
    SELECT
        wga.id,
        wga.user_config_id,
        ap.id
    FROM user_wg_iface_addresses wga
    JOIN address_pools ap
        ON
            host(network(wga.addr))::inet = ap.start_addr
            AND host(broadcast(wga.addr))::inet = ap.end_addr
            AND masklen(wga.addr) = ap.net_mask;

INSERT INTO device_wg_iface_addresses (id, device_id, addr)
    SELECT id, user_config_id, addr FROM user_wg_iface_addresses;

INSERT INTO device_template_wg_iface_dnss (id, device_template_id, dns)
    SELECT id, user_config_id, dns FROM user_wg_iface_dnss;

-- +migrate StatementBegin
DO $$
BEGIN
    IF (SELECT COUNT(*) FROM device_templates) = 0 THEN
        PERFORM setval('device_templates_id_seq', 1, false);
    ELSE
        PERFORM setval('device_templates_id_seq', (SELECT MAX(id) FROM device_templates));
    END IF;
    
    IF (SELECT COUNT(*) FROM devices) = 0 THEN
        PERFORM setval('devices_id_seq', 1, false);
    ELSE
        PERFORM setval('devices_id_seq', (SELECT MAX(id) FROM devices));
    END IF;
    
    IF (SELECT COUNT(*) FROM device_template_address_pools) = 0 THEN
        PERFORM setval('device_template_address_pools_id_seq', 1, false);
    ELSE
        PERFORM setval('device_template_address_pools_id_seq', (SELECT MAX(id) FROM device_template_address_pools));
    END IF;
    
    IF (SELECT COUNT(*) FROM device_wg_iface_addresses) = 0 THEN
        PERFORM setval('device_wg_iface_addresses_id_seq', 1, false);
    ELSE
        PERFORM setval('device_wg_iface_addresses_id_seq', (SELECT MAX(id) FROM device_wg_iface_addresses));
    END IF;
    
    IF (SELECT COUNT(*) FROM device_template_wg_iface_dnss) = 0 THEN
        PERFORM setval('device_template_wg_iface_dnss_id_seq', 1, false);
    ELSE
        PERFORM setval('device_template_wg_iface_dnss_id_seq', (SELECT MAX(id) FROM device_template_wg_iface_dnss));
    END IF;
END $$;
-- +migrate StatementEnd

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
                            SELECT d.public_key
                            FROM device_templates dt
                            JOIN devices d ON dt.id = d.device_template_id
                            WHERE dt.user_id = OLD.user_id
                            LIMIT 1),
                        'newUserPublicKey', (
                            SELECT d.public_key
                            FROM device_templates dt
                            JOIN devices d ON dt.id = d.device_template_id
                            WHERE dt.user_id = NEW.user_id
                            LIMIT 1))
                FROM wg_peer_allowed_ip
                WHERE
                    peer_id = NEW.id
                    AND peer = 'client')::text);
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;
-- +migrate StatementEnd

DROP TABLE user_wg_iface_addresses;
DROP TABLE user_wg_iface_dnss;
DROP TABLE user_wg_configs;

-- +migrate Down

CREATE TABLE user_wg_configs (
    id serial PRIMARY KEY,
    user_id integer REFERENCES users ON DELETE CASCADE NOT NULL,
    interface_name varchar(256) NOT NULL,
    private_key varchar(44) NOT NULL,
    public_key varchar(44) NOT NULL,
    listen_port integer,
    mtu integer
);

CREATE TABLE user_wg_iface_addresses (
    id serial PRIMARY KEY,
    user_config_id integer REFERENCES user_wg_configs ON DELETE CASCADE NOT NULL,
    addr inet NOT NULL
);

CREATE TABLE user_wg_iface_dnss (
    id serial PRIMARY KEY,
    user_config_id integer REFERENCES user_wg_configs ON DELETE CASCADE NOT NULL,
    dns varchar(256) NOT NULL
);

-- If a user has more than one device, delete all except the first
DELETE FROM devices WHERE id IN (
    SELECT b.id FROM devices a
        JOIN devices b USING (device_template_id)
        WHERE a.id < b.id
);

-- Now we know that each user has at most one device. So we can join the tables
-- `device_templates` and `devices` with a simple `JOIN`. (If a user does not
-- have any device, his wg config will be deleted.)
INSERT INTO user_wg_configs (id, user_id, interface_name, private_key, public_key, listen_port, mtu)
    SELECT dt.id, dt.user_id, dt.interface_name, d.private_key, d.public_key, dt.listen_port, dt.mtu
    FROM device_templates dt
    JOIN devices d ON dt.id = d.device_template_id;

-- At this point, addresses of all user's devices except of the first device have been deleted by cascading
INSERT INTO user_wg_iface_addresses (id, user_config_id, addr)
    SELECT wga.id, d.device_template_id, wga.addr
    FROM device_wg_iface_addresses wga
    JOIN devices d ON wga.device_id = d.id;

INSERT INTO user_wg_iface_dnss (id, user_config_id, dns)
    SELECT id, device_template_id, dns FROM device_template_wg_iface_dnss;

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
-- +migrate StatementEnd

-- +migrate StatementBegin
DO $$
BEGIN
    IF (SELECT COUNT(*) FROM user_wg_configs) = 0 THEN
        PERFORM setval('user_wg_configs_id_seq', 1, false);
    ELSE
        PERFORM setval('user_wg_configs_id_seq', (SELECT MAX(id) FROM user_wg_configs));
    END IF;
    
    IF (SELECT COUNT(*) FROM user_wg_iface_addresses) = 0 THEN
        PERFORM setval('user_wg_iface_addresses_id_seq', 1, false);
    ELSE
        PERFORM setval('user_wg_iface_addresses_id_seq', (SELECT MAX(id) FROM user_wg_iface_addresses));
    END IF;
    
    IF (SELECT COUNT(*) FROM user_wg_iface_dnss) = 0 THEN
        PERFORM setval('user_wg_iface_dnss_id_seq', 1, false);
    ELSE
        PERFORM setval('user_wg_iface_dnss_id_seq', (SELECT MAX(id) FROM user_wg_iface_dnss));
    END IF;
END $$;
-- +migrate StatementEnd

DROP TABLE device_template_address_pools;
DROP TABLE device_wg_iface_addresses;
DROP TABLE device_template_wg_iface_dnss;
DROP TABLE devices;
DROP TABLE device_templates;

DELETE FROM address_pools WHERE name LIKE 'auto-generated-%';
