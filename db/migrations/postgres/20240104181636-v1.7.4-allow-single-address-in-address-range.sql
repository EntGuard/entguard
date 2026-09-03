
-- +migrate Up

ALTER DOMAIN inet_range RENAME TO inet_range_old;
CREATE DOMAIN inet_range AS inet_range_type
    CONSTRAINT inet_range_check CHECK ((VALUE).start_addr <= (VALUE).end_addr);

ALTER TABLE ldap_templates
    ALTER COLUMN iface_addr_ranges TYPE inet_range[];

DROP DOMAIN inet_range_old;

-- +migrate Down

ALTER DOMAIN inet_range RENAME TO inet_range_old;
CREATE DOMAIN inet_range AS inet_range_type
    CONSTRAINT inet_range_check CHECK ((VALUE).start_addr < (VALUE).end_addr);

ALTER TABLE ldap_templates
    ALTER COLUMN iface_addr_ranges TYPE inet_range[];

DROP DOMAIN inet_range_old;
