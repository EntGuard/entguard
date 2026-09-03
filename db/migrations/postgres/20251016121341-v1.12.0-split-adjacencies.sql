
-- +migrate Up

CREATE TABLE adjacency_templates (
    id serial PRIMARY KEY,
    server_id integer NOT NULL REFERENCES servers ON DELETE CASCADE,
    user_id integer NOT NULL REFERENCES users ON DELETE CASCADE,
    client_side_allowed_ips cidr[] NOT NULL,
    preshared_key varchar(44)
);

CREATE TABLE adjacencies (
    id serial PRIMARY KEY,
    server_id integer NOT NULL REFERENCES servers ON DELETE CASCADE,
    /* We restrict deleting for device_id. If it had cascade deleting, it would
    cause problems with the trigger trigger_wg_peers_update: at the time the
    adjacency would be about to get cascade-deleted, the trigger would fire, but
    the device would already have been deleted, so the device public key would
    no longer be accessible, so it would be missing in the notification. */
    device_id integer NOT NULL REFERENCES devices ON DELETE RESTRICT,
    server_side_allowed_ips cidr[] NOT NULL,
    client_side_allowed_ips cidr[] NOT NULL,
    preshared_key varchar(44)
);

CREATE TABLE temp_server_side_ips (
    id serial PRIMARY KEY,
    adjacency_id integer,
    ips cidr[]
);

CREATE TABLE temp_client_side_ips (
    id serial PRIMARY KEY,
    adjacency_id integer,
    ips cidr[]
);

INSERT INTO temp_server_side_ips (adjacency_id, ips)
    SELECT peer_id, array_agg(addr)
    FROM wg_peer_allowed_ip
    WHERE peer = 'client' -- 'server' and 'client' were swapped in previous versions
    GROUP BY peer_id;

INSERT INTO temp_client_side_ips (adjacency_id, ips)
    SELECT peer_id, array_agg(addr)
    FROM wg_peer_allowed_ip
    WHERE peer = 'server'
    GROUP BY peer_id;

-- We are introducing NOT NULL constraints, so this is a safeguard against potential NULL values
DELETE FROM wg_peers
    WHERE server_id IS NULL OR user_id IS NULL;

INSERT INTO adjacency_templates (server_id, user_id, client_side_allowed_ips, preshared_key)
    SELECT adj.server_id, adj.user_id, cip.ips, adj.preshared_key
    FROM wg_peers adj
    JOIN temp_client_side_ips cip ON cip.adjacency_id = adj.id;

INSERT INTO adjacencies (server_id, device_id, server_side_allowed_ips, client_side_allowed_ips, preshared_key)
    SELECT adj.server_id, d.id, sip.ips, cip.ips, adj.preshared_key
    FROM wg_peers adj
    JOIN temp_server_side_ips sip ON sip.adjacency_id = adj.id
    JOIN temp_client_side_ips cip ON cip.adjacency_id = adj.id
    JOIN device_templates dt ON dt.user_id = adj.user_id
    JOIN devices d ON d.device_template_id = dt.id;

DROP TABLE temp_server_side_ips;
DROP TABLE temp_client_side_ips;

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
                        'allowedIps', NEW.server_side_allowed_ips,
                        'presharedKey', NEW.preshared_key,
                        'oldDevicePublicKey', (
                            SELECT d.public_key
                            FROM devices d
                            WHERE d.id = OLD.device_id
                        ),
                        'newDevicePublicKey', (
                            SELECT d.public_key
                            FROM devices d
                            WHERE d.id = NEW.device_id
                        )
                    )
        )::text);
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;
-- +migrate StatementEnd

DROP TRIGGER trigger_wg_peers_update ON wg_peers RESTRICT;

CREATE CONSTRAINT TRIGGER trigger_wg_peers_update
    AFTER INSERT OR UPDATE OR DELETE ON adjacencies INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE PROCEDURE notify_wg_peers_update ();

DROP TABLE wg_peer_allowed_ip;
DROP TYPE wg_peer_type;
DROP TABLE wg_peers;

-- +migrate Down

CREATE TABLE wg_peers (
    id serial PRIMARY KEY,
    old_id integer,
    user_id integer REFERENCES users ON DELETE RESTRICT,
    server_id integer REFERENCES servers ON DELETE CASCADE,
    preshared_key varchar(44)
);

ALTER TABLE adjacencies
    DROP CONSTRAINT adjacencies_device_id_fkey,
    ADD CONSTRAINT adjacencies_device_id_fkey FOREIGN KEY (device_id) REFERENCES devices ON DELETE CASCADE;

/* If a user has more than one device, delete all except the first. Then the
corresponding device adjacencies get deleted by cascading. */

DELETE FROM devices WHERE id IN (
    SELECT b.id FROM devices a
        JOIN devices b USING (device_template_id)
        WHERE a.id < b.id
);

/* If a user has multiple devices with adjacencies to the same server, the
following INSERT statement would create multiple adjacencies between the same
user and server, which is error. But we have already deleted excess device
adjacencies, so we are safe. */

INSERT INTO wg_peers (old_id, user_id, server_id, preshared_key)
    SELECT adj.id, dt.user_id, adj.server_id, adj.preshared_key
    FROM adjacencies adj
    JOIN devices d ON d.id = adj.device_id
    JOIN device_templates dt ON dt.id = d.device_template_id;

CREATE TYPE wg_peer_type AS ENUM (
    'client',
    'server'
);

CREATE TABLE wg_peer_allowed_ip (
    id serial PRIMARY KEY,
    peer_id integer REFERENCES wg_peers ON DELETE CASCADE,
    addr cidr NOT NULL,
    peer wg_peer_type NOT NULL
);

INSERT INTO wg_peer_allowed_ip (peer_id, addr, peer)
    SELECT wg_peers.id, unnest(adj.server_side_allowed_ips), 'client' -- 'server' and 'client' were swapped in previous versions
    FROM adjacencies adj
    JOIN wg_peers ON wg_peers.old_id = adj.id;

INSERT INTO wg_peer_allowed_ip (peer_id, addr, peer)
    SELECT wg_peers.id, unnest(adj.client_side_allowed_ips), 'server'
    FROM adjacencies adj
    JOIN wg_peers ON wg_peers.old_id = adj.id;

ALTER TABLE wg_peers
    DROP COLUMN old_id;

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

DROP TRIGGER trigger_wg_peers_update ON adjacencies RESTRICT;

CREATE CONSTRAINT TRIGGER trigger_wg_peers_update
    AFTER INSERT OR UPDATE OR DELETE ON wg_peers INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE PROCEDURE notify_wg_peers_update ();

DROP TABLE adjacency_templates;
DROP TABLE adjacencies;
