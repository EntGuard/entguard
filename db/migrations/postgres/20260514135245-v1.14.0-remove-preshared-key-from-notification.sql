
-- +migrate Up

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
                        'adjacencyId', NEW.id,
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

-- +migrate Down

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
