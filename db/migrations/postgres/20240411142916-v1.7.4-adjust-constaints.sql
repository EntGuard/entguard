
-- +migrate Up

ALTER TABLE wg_peers
    DROP CONSTRAINT wg_peers_user_id_fkey,
    ADD CONSTRAINT wg_peers_user_id_fkey
        FOREIGN KEY (user_id)
        REFERENCES users ON DELETE RESTRICT;

ALTER TABLE ldap_template_servers
    DROP CONSTRAINT ldap_template_servers_ldap_template_id_fkey,
    ADD CONSTRAINT ldap_template_servers_ldap_template_id_fkey
        FOREIGN KEY (ldap_template_id)
        REFERENCES ldap_templates ON DELETE CASCADE;

-- +migrate Down

ALTER TABLE wg_peers
    DROP CONSTRAINT wg_peers_user_id_fkey,
    ADD CONSTRAINT wg_peers_user_id_fkey
        FOREIGN KEY (user_id)
        REFERENCES users ON DELETE CASCADE;

ALTER TABLE ldap_template_servers
    DROP CONSTRAINT ldap_template_servers_ldap_template_id_fkey,
    ADD CONSTRAINT ldap_template_servers_ldap_template_id_fkey
        FOREIGN KEY (ldap_template_id)
        REFERENCES ldap_templates;
