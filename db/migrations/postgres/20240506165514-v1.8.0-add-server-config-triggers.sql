
-- +migrate Up

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
    IF NOT EXISTS (SELECT FROM pg_trigger WHERE tgname = 'trigger_wg_configs_update') THEN
        CREATE CONSTRAINT TRIGGER trigger_wg_configs_update
        AFTER UPDATE ON wg_configs INITIALLY DEFERRED
        FOR EACH ROW
        WHEN (NEW.server_id is not null)
        EXECUTE PROCEDURE notify_wg_configs_update ();
    END IF;
END
$$;
-- +migrate StatementEnd

-- +migrate Down
DROP TRIGGER IF EXISTS trigger_wg_configs_update ON wg_configs restrict;
DROP FUNCTION IF EXISTS notify_wg_configs_update restrict;
