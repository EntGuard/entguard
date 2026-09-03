
-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION notify_user_session_update ()
    RETURNS TRIGGER
    AS $$
DECLARE
BEGIN
    PERFORM
        pg_notify('session_updated', (
                json_build_object(
                    'oldSession', OLD.session_id,
                    'newSession', NEW.session_id,
                    'userID', OLD.id))::text);
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_trigger WHERE tgname = 'trigger_sessions_update') THEN
        CREATE CONSTRAINT TRIGGER trigger_sessions_update
        AFTER DELETE OR UPDATE OF session_id ON users INITIALLY DEFERRED
        FOR EACH ROW EXECUTE PROCEDURE notify_user_session_update ();
    END IF;
END
$$;
-- +migrate StatementEnd

-- +migrate Down
DROP TRIGGER IF EXISTS trigger_sessions_update ON users restrict;
DROP FUNCTION IF EXISTS notify_user_session_update restrict;
