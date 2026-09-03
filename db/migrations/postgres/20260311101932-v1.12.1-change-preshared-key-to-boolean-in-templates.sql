
-- +migrate Up

ALTER TABLE ldap_template_servers
    ADD use_preshared_key boolean;
ALTER TABLE adjacency_templates
    ADD use_preshared_key boolean;

UPDATE ldap_template_servers
    SET use_preshared_key = (COALESCE(preshared_key, '') != '');
UPDATE adjacency_templates
    SET use_preshared_key = (COALESCE(preshared_key, '') != '');

ALTER TABLE ldap_template_servers
    ALTER COLUMN use_preshared_key SET NOT NULL;
ALTER TABLE adjacency_templates
    ALTER COLUMN use_preshared_key SET NOT NULL;

ALTER TABLE ldap_template_servers
    DROP COLUMN preshared_key;
ALTER TABLE adjacency_templates
    DROP COLUMN preshared_key;

-- +migrate Down

ALTER TABLE ldap_template_servers
    ADD preshared_key varchar(44);
ALTER TABLE adjacency_templates
    ADD preshared_key varchar(44);

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Before the migration, if preshared_key was not set, the table
-- ldap_template_servers used '' and the table adjacency_templates used NULL
UPDATE ldap_template_servers
    SET preshared_key = encode(gen_random_bytes(32), 'base64')
    WHERE use_preshared_key = true;
UPDATE ldap_template_servers
    SET preshared_key = ''
    WHERE use_preshared_key = false;
UPDATE adjacency_templates
    SET preshared_key = encode(gen_random_bytes(32), 'base64')
    WHERE use_preshared_key = true;
UPDATE adjacency_templates
    SET preshared_key = NULL
    WHERE use_preshared_key = false;

DROP EXTENSION IF EXISTS pgcrypto;

ALTER TABLE ldap_template_servers
    DROP COLUMN use_preshared_key;
ALTER TABLE adjacency_templates
    DROP COLUMN use_preshared_key;
