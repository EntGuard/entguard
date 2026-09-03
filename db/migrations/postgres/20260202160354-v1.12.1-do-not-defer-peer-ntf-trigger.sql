
-- +migrate Up

DROP TRIGGER trigger_wg_peers_update ON adjacencies RESTRICT;

CREATE CONSTRAINT TRIGGER trigger_wg_peers_update
    AFTER INSERT OR UPDATE OR DELETE ON adjacencies
    FOR EACH ROW
    EXECUTE PROCEDURE notify_wg_peers_update ();

-- +migrate Down

DROP TRIGGER trigger_wg_peers_update ON adjacencies RESTRICT;

CREATE CONSTRAINT TRIGGER trigger_wg_peers_update
    AFTER INSERT OR UPDATE OR DELETE ON adjacencies INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE PROCEDURE notify_wg_peers_update ();
