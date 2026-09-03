-- +migrate Up

CREATE TABLE ldap_template_address_pools (
    id serial PRIMARY KEY,
    ldap_template_id integer REFERENCES ldap_templates ON DELETE CASCADE NOT NULL,
    address_pool_id integer REFERENCES address_pools ON DELETE RESTRICT NOT NULL
);

CREATE SEQUENCE ldap_ap;

-- Create address pools from LDAP template address ranges
INSERT INTO address_pools (name, description, start_addr, end_addr, net_mask)
    SELECT
        'ldap-auto-generated-' || nextval('ldap_ap'),
        'automatically generated from LDAP template "' || lt.name || '" during database migration',
        host(addr_range.start_addr)::inet,
        host(addr_range.end_addr)::inet,
        COALESCE(masklen(addr_range.start_addr), 24)
    FROM ldap_templates lt
    CROSS JOIN LATERAL unnest(lt.iface_addr_ranges) AS addr_range;

-- Create address pools from LDAP template CIDR addresses  
INSERT INTO address_pools (name, description, start_addr, end_addr, net_mask)
    SELECT
        'ldap-auto-generated-' || nextval('ldap_ap'),
        'automatically generated from LDAP template "' || lt.name || '" CIDR during database migration',
        host(network(cidr_addr))::inet,
        host(broadcast(cidr_addr))::inet,
        masklen(cidr_addr)
    FROM ldap_templates lt
    CROSS JOIN LATERAL unnest(lt.iface_addrs) AS cidr_addr;

-- Remove duplicate address pools from previous insertions
DELETE FROM address_pools WHERE id IN (
    SELECT b.id FROM address_pools a
        JOIN address_pools b USING (start_addr, end_addr, net_mask)
        WHERE a.id < b.id AND a.name LIKE 'ldap-auto-generated-%'
);

DROP SEQUENCE ldap_ap;

-- Link LDAP templates to address pools created from address ranges
INSERT INTO ldap_template_address_pools (ldap_template_id, address_pool_id)
    SELECT DISTINCT
        lt.id,
        ap.id
    FROM ldap_templates lt
    CROSS JOIN LATERAL unnest(lt.iface_addr_ranges) AS addr_range
    JOIN address_pools ap 
        ON ap.start_addr = host(addr_range.start_addr)::inet
        AND ap.end_addr = host(addr_range.end_addr)::inet
        AND ap.name LIKE 'ldap-auto-generated-%';

-- Link LDAP templates to address pools created from CIDR addresses
INSERT INTO ldap_template_address_pools (ldap_template_id, address_pool_id)
    SELECT DISTINCT
        lt.id,
        ap.id
    FROM ldap_templates lt
    CROSS JOIN LATERAL unnest(lt.iface_addrs) AS cidr_addr
    JOIN address_pools ap 
        ON ap.start_addr = host(network(cidr_addr))::inet
        AND ap.end_addr = host(broadcast(cidr_addr))::inet
        AND ap.net_mask = masklen(cidr_addr)
        AND ap.name LIKE 'ldap-auto-generated-%';

ALTER TABLE ldap_templates DROP COLUMN iface_addr_ranges;
ALTER TABLE ldap_templates DROP COLUMN iface_addrs;
ALTER TABLE ldap_templates RENAME COLUMN iface_name TO interface_name;
ALTER TABLE ldap_templates ALTER COLUMN interface_name SET NOT NULL;


-- +migrate Down

ALTER TABLE ldap_templates ADD COLUMN iface_addr_ranges inet_range[];
ALTER TABLE ldap_templates ADD COLUMN iface_addrs cidr[];
ALTER TABLE ldap_templates ALTER COLUMN interface_name DROP NOT NULL;
ALTER TABLE ldap_templates RENAME COLUMN interface_name TO iface_name;

UPDATE ldap_templates 
SET iface_addr_ranges = restored_ranges.ranges
FROM (
    SELECT 
        ltap.ldap_template_id,
        array_agg(ROW(set_masklen(ap.start_addr, ap.net_mask), set_masklen(ap.end_addr, ap.net_mask))::inet_range) AS ranges
    FROM ldap_template_address_pools ltap
    JOIN address_pools ap ON ltap.address_pool_id = ap.id
    WHERE ap.name LIKE 'ldap-auto-generated-%' 
    AND ap.description LIKE '%during database migration' 
    AND ap.description NOT LIKE '%CIDR%'
    AND ap.start_addr != host(network(set_masklen(ap.start_addr, ap.net_mask)))::inet
    AND ap.end_addr != host(broadcast(set_masklen(ap.start_addr, ap.net_mask)))::inet
    GROUP BY ltap.ldap_template_id
) AS restored_ranges
WHERE ldap_templates.id = restored_ranges.ldap_template_id;

UPDATE ldap_templates 
SET iface_addrs = restored_cidrs.cidrs
FROM (
    SELECT 
        ltap.ldap_template_id,
        array_agg(set_masklen(ap.start_addr, ap.net_mask)) AS cidrs
    FROM ldap_template_address_pools ltap
    JOIN address_pools ap ON ltap.address_pool_id = ap.id
    WHERE ap.name LIKE 'ldap-auto-generated-%' 
    AND ap.description LIKE '%CIDR%'
    GROUP BY ltap.ldap_template_id
) AS restored_cidrs
WHERE ldap_templates.id = restored_cidrs.ldap_template_id;

DROP TABLE ldap_template_address_pools;

DELETE FROM address_pools WHERE name LIKE 'ldap-auto-generated-%';
