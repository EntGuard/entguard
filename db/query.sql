-- name: GetAllUsers :many
SELECT id, username, is_admin, auth_service, mfa_auth, ntf, updated_at
FROM users
ORDER BY id;

-- name: GetUserByUsername :one
SELECT id, password, is_admin, auth_service, mfa_auth, ntf, updated_at
FROM users
WHERE username=$1;

-- name: GetUserByID :one
SELECT username, password, is_admin, auth_service, mfa_auth, ntf, updated_at
FROM users
WHERE id=$1;

-- name: UsernameCheck :one 
SELECT 1 FROM users
WHERE username=$1
LIMIT 1;

-- name: UserIDCheck :one 
SELECT 1 FROM users
WHERE id=$1
LIMIT 1;

-- name: UserExists :one
SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 LIMIT 1);

-- name: InsertUser :one
INSERT INTO users (username, password, is_admin, auth_service, mfa_auth, ntf, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET username=$1, is_admin=$2, mfa_auth=$3, ntf=$4, updated_at=$5 
WHERE id=$6
RETURNING *;

-- name: UpdateUserPwd :one 
UPDATE users
SET password=$1, updated_at=$2
WHERE id=$3
RETURNING *;

-- name: UpdateUserEditTime :one 
UPDATE users
SET updated_at=$1
WHERE id=$2
RETURNING *;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id=$1;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: GetUserTOTP :one
SELECT key_encrypted
FROM totp
WHERE user_id=$1;

-- name: InsertUserTOTP :one
INSERT INTO totp (key_encrypted, user_id)
VALUES ($1, $2)
RETURNING *;

-- name: UpdateUserTOTP :one
UPDATE totp
SET key_encrypted=$1
WHERE user_id=$2
RETURNING *;

-- name: GetMFACertsByType :many
SELECT id, cert
FROM mfa_certs
WHERE cert_type=$1;

-- name: InsertMFACert :one
INSERT INTO mfa_certs(cert, cert_type)
VALUES ($1, $2)
RETURNING *;

-- name: DeleteMFACert :exec
DELETE FROM mfa_certs WHERE id=$1;

-- name: GetAllLdapConfigs :many
SELECT
    id, priority, host, port, base_dn, bind_dn, bind_pw_encrypted,
    user_list_filter, username_attribute, uid_attribute,
    use_tls, fqdn, ca_cert, COALESCE(template_id, 0)
FROM ldaps
ORDER BY priority;

-- name: GetLdapConfigByID :one
SELECT
    priority, host, port, base_dn, bind_dn, bind_pw_encrypted,
    user_list_filter, username_attribute, uid_attribute,
    use_tls, fqdn, ca_cert, COALESCE(template_id, 0)
FROM ldaps
WHERE id=$1;

-- name: LdapConfigVerifyPriority :one
SELECT 1 FROM ldaps WHERE priority=$1;

-- name: InsertLdapConfig :one
INSERT INTO ldaps (
    priority, host, port, base_dn,
    bind_dn, bind_pw_encrypted,
    user_list_filter, username_attribute, uid_attribute,
    use_tls, fqdn, ca_cert, template_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: LdapTemplateNameCheck :one
SELECT EXISTS(SELECT 1 FROM ldap_templates WHERE name=$1 LIMIT 1);

-- name: LdapTemplateIDCheck :one
SELECT EXISTS(SELECT 1 FROM ldap_templates WHERE id=$1 LIMIT 1);

-- name: GetAllLdapTemplates :many
SELECT * FROM ldap_templates;

-- name: GetLdapTemplateByID :one
SELECT * FROM ldap_templates
WHERE id=$1;

-- name: InsertLdapTemplate :one
INSERT INTO ldap_templates (
    name, filter, interface_name, is_admin, mfa_auth, listen_port, mtu, dns
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateLdapTemplate :one
UPDATE ldap_templates
SET name=$1, filter=$2, interface_name=$3, is_admin=$4, mfa_auth=$5, listen_port=$6, mtu=$7, dns=$8
WHERE id=$9
RETURNING *;

-- name: DeleteLdapTemplate :exec
DELETE FROM ldap_templates WHERE id=$1;

-- name: InsertLdapTemplateServer :one
INSERT INTO ldap_template_servers (
    ldap_template_id, server_id, allowed_ips, use_preshared_key
)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateLdapTemplateServer :one
UPDATE ldap_template_servers
SET server_id = $1,
allowed_ips = $2, use_preshared_key = $3
WHERE ldap_template_id = $4 AND server_id = $5
RETURNING *;

-- name: DeleteLdapTemplateServer :exec
DELETE FROM ldap_template_servers
WHERE ldap_template_id = $1
AND server_id = $2;

-- name: GetLdapTemplateServers :many
SELECT servers.name, server_id, ldap_template_id, allowed_ips, use_preshared_key
FROM ldap_template_servers
LEFT JOIN servers
ON ldap_template_servers.server_id = servers.id
WHERE ldap_template_servers.ldap_template_id=$1;

-- name: GetLdapTemplateServersByServerID :many
SELECT ldap_template_id, server_id, allowed_ips, use_preshared_key
FROM ldap_template_servers
WHERE server_id=$1;

-- name: UpdateLdapTemplateServerAllowedIPs :exec
UPDATE ldap_template_servers
SET allowed_ips=$3
WHERE ldap_template_id=$1 AND server_id=$2;

-- name: UpdateLdapConfig :one
UPDATE ldaps
SET
    priority=$1, host=$2, port=$3, base_dn=$4,
    bind_dn=$5, bind_pw_encrypted=$6,
    user_list_filter=$7, username_attribute=$8, uid_attribute=$9,
    use_tls=$10, fqdn=$11, ca_cert=$12, template_id=$13
WHERE id=$14
RETURNING *;

-- name: DeleteLdapConfig :exec
DELETE FROM ldaps WHERE id=$1;

-- name: GetLdapUserAuths :many
SELECT ldaps.priority, ldaps.host, ldaps.port, users_to_ldaps.user_dn, ldaps.use_tls, ldaps.fqdn, ldaps.ca_cert
FROM users_to_ldaps
JOIN ldaps ON users_to_ldaps.ldap_id=ldaps.id
WHERE users_to_ldaps.user_id=$1
ORDER BY ldaps.priority;

-- name: GetLdapUsersPriorities :many
SELECT ldaps.priority, users_to_ldaps.user_id, users_to_ldaps.ldap_id
FROM users_to_ldaps
JOIN ldaps ON users_to_ldaps.ldap_id=ldaps.id
ORDER BY ldaps.priority;

-- name: GetLdapRelationByLdapIdAndUserID :one
-- struct_name: UsersToLdaps
SELECT id, user_id, user_dn, created_at, updated_at
FROM users_to_ldaps
WHERE ldap_id=$1
AND user_uid=$2;

-- name: InsertLdapRelation :one
-- struct_name: UsersToLdaps
INSERT INTO users_to_ldaps (user_id, ldap_id, user_uid, user_dn, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateLdapRelation :one
-- struct_name: UsersToLdaps
UPDATE users_to_ldaps
SET user_id=$1, user_dn=$2, updated_at=$3
WHERE id=$4
AND ldap_id=$5
AND user_uid=$6
RETURNING *;

-- name: DeleteLdapRelations :exec
DELETE FROM users_to_ldaps
WHERE updated_at < $1;

-- name: GetLdapUsersWithoutRelations :many
SELECT id, username
FROM users
WHERE id IN (
    SELECT users.id
    FROM users
    LEFT JOIN users_to_ldaps ON users.id=users_to_ldaps.user_id
    WHERE users.auth_service='ldap'
    GROUP BY users.id
    HAVING COUNT(users_to_ldaps.id) = 0
);


-- name: ServerIDCheck :one
SELECT 1 FROM servers
WHERE id=$1
LIMIT 1;

-- name: ServerNameCheck :one
SELECT 1 FROM servers
WHERE name=$1
LIMIT 1;

-- name: GetAllServers :many
SELECT id, name, endpoint, description, healthcheck_address
FROM servers;

-- name: GetServerByID :one
SELECT name, endpoint, description, healthcheck_address
FROM servers
WHERE id=$1;

-- name: GetServerByName :one
SELECT id, endpoint, description, healthcheck_address
FROM servers
WHERE name=$1;

-- name: InsertServer :one
INSERT INTO servers (name, endpoint, description, healthcheck_address)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateServer :one
UPDATE servers SET name=$1, endpoint=$2, description=$3, healthcheck_address=$4
WHERE id=$5
RETURNING *;

-- name: DeleteServer :exec
DELETE FROM servers WHERE id=$1;

-- name: GetServerReplicas :many
SELECT running_replicas
FROM servers
WHERE id=$1;

-- name: IncreaseServerReplicas :one
UPDATE servers 
SET running_replicas = running_replicas + 1
WHERE id=$1
RETURNING *;

-- name: DecreaseServerReplicas :one
UPDATE servers 
SET running_replicas = running_replicas - 1
WHERE running_replicas > 0 
AND id=$1
RETURNING *;

-- name: GetServerTags :many
SELECT tag FROM tags
WHERE server_id=$1;

-- name: InsertServerTag :one
INSERT INTO tags (tag, created, server_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetServerTagsByName :many
SELECT tag FROM tags
WHERE server_id IN (
    SELECT id FROM servers WHERE name=$1)
ORDER BY created DESC;

-- name: DeleteTags :exec
TRUNCATE TABLE tags;

-- name: GetCA :one
SELECT cert, key_encrypted
FROM root_ca
LIMIT 1;

-- name: DeleteCA :exec
TRUNCATE TABLE root_ca;

-- name: InsertCA :one
INSERT INTO root_ca (cert, key_encrypted) VALUES ($1, $2)
RETURNING *;

-- name: DeviceTemplateExists :one
SELECT 1 FROM device_templates 
WHERE id=$1;

-- name: GetDeviceTemplateByID :one
SELECT
    id,
    user_id,
    interface_name,
    COALESCE(listen_port, -1),
    COALESCE(mtu, -1)
FROM device_templates
WHERE id=$1;

-- name: GetDeviceTemplateByUserID :one
SELECT
    id,
    user_id,
    interface_name,
    COALESCE(listen_port, -1),
    COALESCE(mtu, -1)
FROM device_templates
WHERE user_id=$1;

-- name: GetServerWireGuardConfig :one
SELECT
    id,
    interface_name,
    private_key_encrypted,
    public_key,
    COALESCE(listen_port, -1),
    COALESCE(mtu, -1),
    COALESCE(persistent_keepalive, -1)
FROM server_wg_configs
WHERE server_id=$1;

-- name: GetAdjacency :one
SELECT * FROM adjacencies WHERE id=$1;

-- name: GetDeviceWireGuardAdjacencies :many
SELECT
    adjacencies.id,
    adjacencies.server_side_allowed_ips,
    adjacencies.client_side_allowed_ips,
    COALESCE(adjacencies.preshared_key_encrypted, ''),
    COALESCE(server_wg_configs.persistent_keepalive, -1),
    COALESCE(server_wg_configs.listen_port, -1),
    server_wg_configs.public_key,
    servers.id AS serverID,
    servers.name AS serverName,
    servers.endpoint AS serverEndpoint,
    servers.description AS serverDescription,
    servers.healthcheck_address AS serverHealthcheckAddress
FROM adjacencies
JOIN servers ON adjacencies.server_id=servers.id
JOIN server_wg_configs ON adjacencies.server_id=server_wg_configs.server_id
WHERE adjacencies.device_id=$1;

-- name: GetServerWireGuardAdjacencies :many
SELECT
    adjacencies.id,
    adjacencies.device_id,
    adjacencies.server_side_allowed_ips,
    adjacencies.client_side_allowed_ips,
    COALESCE(adjacencies.preshared_key_encrypted, ''),
    COALESCE(dt.listen_port, -1),
    d.public_key
FROM adjacencies
JOIN devices d ON adjacencies.device_id = d.id
JOIN device_templates dt ON dt.id = d.device_template_id
WHERE adjacencies.server_id=$1;

-- name: AdjacencyExists :one
SELECT EXISTS(
    SELECT 1 FROM adjacencies
    WHERE server_id=$1
    AND device_id=$2
);

-- name: AdjacencyTemplateExists :one
SELECT EXISTS(
    SELECT 1 FROM adjacency_templates
    WHERE server_id=$1
    AND user_id=$2
);

-- name: AdjacencyTemplateExistsByID :one
SELECT EXISTS(
    SELECT 1 FROM adjacency_templates
    WHERE id=$1
);

-- name: GetAdjacencyTemplateByID :one
SELECT * FROM adjacency_templates
WHERE id=$1;

-- name: GetUserAdjacencyTemplates :many
SELECT
    adjacency_templates.id,
    adjacency_templates.server_id,
    adjacency_templates.user_id,
    adjacency_templates.client_side_allowed_ips,
    adjacency_templates.use_preshared_key,
    servers.name AS server_name,
    servers.endpoint AS server_endpoint,
    servers.description AS server_description,
    servers.healthcheck_address AS server_healthcheck_address
FROM adjacency_templates
JOIN servers ON adjacency_templates.server_id=servers.id
WHERE adjacency_templates.user_id=$1;

-- name: InsertAdjacencyTemplate :one
INSERT INTO adjacency_templates(server_id, user_id, client_side_allowed_ips, use_preshared_key)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateAdjacencyTemplate :one
UPDATE adjacency_templates
SET server_id=$2, client_side_allowed_ips=$3, use_preshared_key=$4
WHERE id=$1
RETURNING *;

-- name: DeleteAdjacencyTemplate :exec
DELETE FROM adjacency_templates
WHERE id=$1;

-- name: GetAdjacencyTemplatesByServerID :many
SELECT * FROM adjacency_templates
WHERE server_id=$1;

-- name: GetAdjacenciesByServerID :many
SELECT * FROM adjacencies
WHERE server_id=$1;

-- name: UpdateAdjacencyClientSideAllowedIPs :exec
UPDATE adjacencies
SET client_side_allowed_ips=$2
WHERE id=$1;

-- name: UpdateAdjacencyTemplateClientSideAllowedIPs :exec
UPDATE adjacency_templates
SET client_side_allowed_ips=$2
WHERE id=$1;

-- name: GetServerWireGuardIfaceAddrs :many
SELECT addr
FROM server_wg_iface_addresses
WHERE server_config_id=$1;

-- name: GetServerWireGuardIfaceDNSs :many
SELECT dns
FROM server_wg_iface_dnss
WHERE server_config_id=$1;

-- name: GetWireGuardIfaceAllAddrs :many
SELECT addr FROM device_wg_iface_addresses
UNION ALL
SELECT addr FROM server_wg_iface_addresses;

-- name: GetDeviceTemplateAddressPoolIDs :many
SELECT address_pool_id
FROM device_template_address_pools
WHERE device_template_id=$1;

-- name: GetDeviceWireGuardIfaceAddrs :many
SELECT addr
FROM device_wg_iface_addresses
WHERE device_id=$1;

-- name: GetDeviceTemplateWireGuardIfaceDNSs :many
SELECT dns
FROM device_template_wg_iface_dnss
WHERE device_template_id=$1;

-- name: InsertDeviceTemplate :one
INSERT INTO device_templates(user_id, interface_name, listen_port, mtu, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
RETURNING *;

-- name: InsertServerWireGuardConfig :one
INSERT INTO server_wg_configs(server_id, interface_name, private_key_encrypted, public_key, listen_port, mtu, persistent_keepalive)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: InsertDeviceTemplateAddressPool :one
INSERT INTO device_template_address_pools(device_template_id, address_pool_id)
VALUES ($1, $2)
RETURNING *;

-- name: InsertDeviceWireGuardIfaceAddr :one
INSERT INTO device_wg_iface_addresses(device_id, addr)
VALUES ($1, $2)
RETURNING *;

-- name: InsertServerWireGuardIfaceAddr :one
INSERT INTO server_wg_iface_addresses(server_config_id, addr)
VALUES ($1, $2)
RETURNING *;

-- name: InsertDeviceTemplateWireGuardIfaceDNS :one
INSERT INTO device_template_wg_iface_dnss(device_template_id, dns)
VALUES ($1, $2)
RETURNING *;

-- name: InsertServerWireGuardIfaceDNS :one
INSERT INTO server_wg_iface_dnss(server_config_id, dns)
VALUES ($1, $2)
RETURNING *;

-- name: InsertAdjacency :one
INSERT INTO adjacencies(server_id, device_id, server_side_allowed_ips, client_side_allowed_ips, preshared_key_encrypted)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateDeviceTemplateByUserID :one
UPDATE device_templates
SET interface_name=$2, listen_port=$3, mtu=$4
WHERE user_id=$1
RETURNING *;

-- name: UpdateServerWireGuardConfig :one
UPDATE server_wg_configs
SET interface_name=$2, private_key_encrypted=$3, public_key=$4, listen_port=$5, mtu=$6, persistent_keepalive=$7
WHERE server_id=$1
RETURNING *;

-- name: UpdateAdjacency :one
UPDATE adjacencies
SET server_id=$2, device_id=$3, preshared_key_encrypted=$4,
    server_side_allowed_ips=COALESCE($5, server_side_allowed_ips),
    client_side_allowed_ips=$6
WHERE id=$1
RETURNING *;

-- name: DeleteDeviceTemplate :exec
DELETE FROM device_templates
WHERE id=$1;

-- name: DeleteServerWireGuardConfig :exec
DELETE FROM server_wg_configs
WHERE server_id=$1;

-- name: DeleteAdjacency :exec
DELETE FROM adjacencies
WHERE id=$1;

-- name: DeleteDeviceTemplateAddressPools :exec
DELETE FROM device_template_address_pools
WHERE device_template_id=$1;

-- name: DeleteDeviceWireGuardIfaceAddrs :exec
DELETE FROM device_wg_iface_addresses
WHERE device_id=$1;

-- name: DeleteServerWireGuardIfaceAddrs :exec
DELETE FROM server_wg_iface_addresses
WHERE server_config_id=$1;

-- name: DeleteDeviceTemplateWireGuardDNS :exec
DELETE FROM device_template_wg_iface_dnss
WHERE device_template_id=$1;

-- name: DeleteServerWireGuardDNS :exec
DELETE FROM server_wg_iface_dnss
WHERE server_config_id=$1;

-- name: IsHealthCheckEnabledForDevice :one
SELECT EXISTS(
    SELECT 1 FROM adjacencies AS a
    JOIN servers AS s
        ON a.server_id=s.id
    WHERE a.device_id=$1
        AND s.healthcheck_address IS NOT NULL
    LIMIT 1
);

-- name: InsertAddressPool :one
INSERT INTO address_pools (
    name, description, start_addr, end_addr, net_mask
)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAddressPoolByID :one
SELECT * FROM address_pools
WHERE id = $1;

-- name: GetAddressPoolByName :one
SELECT * FROM address_pools
WHERE name = $1;

-- name: GetAllAddressPools :many
SELECT * FROM address_pools
ORDER BY start_addr;

-- name: UpdateAddressPool :one
UPDATE address_pools
SET
    name = $2,
    description = $3,
    start_addr = $4,
    end_addr = $5,
    net_mask = $6,
    updated_at = $7
WHERE id = $1
RETURNING *;

-- name: DeleteAddressPool :one
DELETE FROM address_pools
WHERE id = $1
RETURNING id;

-- name: InsertDevice :one
INSERT INTO devices(description, device_template_id, external_device_id, private_key_encrypted, public_key, device_information, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now(), now())
RETURNING *;

-- name: GetDeviceByID :one
SELECT * FROM devices WHERE id=$1;

-- name: GetDeviceByExternalID :one
SELECT * FROM devices WHERE external_device_id=$1;

-- name: GetDeviceByExternalIDAndUserID :one
SELECT d.* FROM devices d
JOIN device_templates dt ON d.device_template_id = dt.id
WHERE d.external_device_id=$1 AND d.external_device_id!='' AND dt.user_id=$2;

-- name: GetDeviceWithNoExternalIDByUserID :one
SELECT d.* FROM devices d
JOIN device_templates dt ON d.device_template_id = dt.id
WHERE d.external_device_id='' AND dt.user_id=$1
LIMIT 1;

-- name: GetDeviceWithNoSessionIDOrderedByTimestamp :one
SELECT d.* FROM devices d
JOIN device_templates dt ON d.device_template_id = dt.id
WHERE d.session_id='' AND dt.user_id=$1
ORDER BY d.last_time_connected ASC NULLS FIRST
LIMIT 1;

-- name: GetDeviceWithOldestTimestampByUserID :one
SELECT d.* FROM devices d
JOIN device_templates dt ON d.device_template_id = dt.id
WHERE dt.user_id=$1
ORDER BY d.last_time_connected ASC NULLS FIRST
LIMIT 1;

-- name: UpdateDeviceFromConfiguration :one
UPDATE devices
SET external_device_id=$2, session_id=$3, last_time_connected=$4, device_information=$5, updated_at=now()
WHERE id=$1
RETURNING *;

-- name: GetSessionByDeviceID :one
SELECT session_id, last_time_connected FROM devices WHERE id=$1;

-- name: GetAllSessions :many
SELECT session_id FROM devices WHERE session_id != '' AND session_id IS NOT NULL;

-- name: GetAllDevices :many
SELECT * FROM devices ORDER BY id;

-- name: UpdateDevice :one
UPDATE devices
SET description=$2, private_key_encrypted=$3, public_key=$4,
    updated_at=now()
WHERE id=$1
RETURNING *;

-- name: UpdateDeviceSessionAndLastTimeConnected :one
UPDATE devices
SET session_id=$2, last_time_connected=$3,
    updated_at=now()
WHERE id=$1
RETURNING *;

-- name: DeleteDevice :execrows
DELETE FROM devices WHERE id=$1;

-- name: InvalidateDeviceSession :one
UPDATE devices
SET session_id='', updated_at=now()
WHERE id=$1
RETURNING *;

-- name: InvalidateTemplateDeviceSessions :execrows
UPDATE devices
SET session_id='', updated_at=now()
WHERE device_template_id=$1;

-- name: InvalidateOldDeviceSessions :execrows
UPDATE devices
SET session_id='', updated_at=now() 
WHERE last_time_connected < $1;

-- name: InvalidateAllDeviceSessionsWithCert :execrows
WITH t AS (
    SELECT d.id AS row_id
    FROM devices d
    JOIN device_templates dt ON d.device_template_id = dt.id
    JOIN users u ON dt.user_id = u.id
    WHERE u.mfa_auth = 'cert'
)
UPDATE devices
SET session_id='', updated_at=now()
FROM t
WHERE id = t.row_id;

-- name: InvalidateAllDeviceSessionsByServerID :execrows
WITH t AS (
    SELECT d.id AS row_id
    FROM devices d
    JOIN adjacencies adj ON adj.device_id = d.id
    WHERE adj.server_id = $1
)
UPDATE devices
SET session_id='', updated_at=now()
FROM t
WHERE id = t.row_id;

-- name: GetDeviceByUserID :one
SELECT d.* FROM devices d
JOIN device_templates dt ON d.device_template_id = dt.id
WHERE dt.user_id=$1
LIMIT 1;

-- name: GetDevicesByUserID :many
SELECT d.* FROM devices d
JOIN device_templates dt ON d.device_template_id = dt.id
WHERE dt.user_id=$1;

-- name: GetDevicesByTemplateID :many
SELECT * FROM devices WHERE device_template_id=$1;

-- name: GetDeviceTemplateAddressPools :many
SELECT ap.* FROM address_pools ap
JOIN device_template_address_pools dtap ON ap.id = dtap.address_pool_id
WHERE dtap.device_template_id=$1;

-- name: GetLdapTemplateAddressPools :many
SELECT ap.*
FROM address_pools ap
JOIN ldap_template_address_pools ltap ON ap.id = ltap.address_pool_id
WHERE ltap.ldap_template_id = $1;

-- name: InsertLdapTemplateAddressPool :exec
INSERT INTO ldap_template_address_pools (ldap_template_id, address_pool_id)
VALUES ($1, $2);

-- name: DeleteLdapTemplateAddressPools :exec
DELETE FROM ldap_template_address_pools WHERE ldap_template_id = $1;
