
-- +migrate Up

ALTER TABLE ldaps
    ALTER COLUMN base_dn TYPE text,
    ALTER COLUMN bind_dn TYPE text,
    ALTER COLUMN bind_pw TYPE varchar(72);

-- +migrate Down

UPDATE ldaps
SET base_dn = base_dn::varchar(40),
    bind_dn = bind_dn::varchar(40),
    bind_pw = bind_pw::varchar(40);

ALTER TABLE ldaps
    ALTER COLUMN base_dn TYPE varchar(40),
    ALTER COLUMN bind_dn TYPE varchar(40),
    ALTER COLUMN bind_pw TYPE varchar(40);
