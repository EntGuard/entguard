-- +migrate Up

CREATE TABLE address_pools (
    id SERIAL PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    start_addr INET NOT NULL,
    end_addr INET NOT NULL,
    net_mask INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

-- +migrate Down

DROP TABLE address_pools;