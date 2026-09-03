/*
 * Copyright 2020 PANTHEON.tech s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package api

import (
	"encoding/json"
	"time"
)

// List of REST endpoints.
const (
	// Get list of servers:
	// 		GET /api/v1/servers
	// 			in: <empty>.
	// 			out: `ServerList` as json.
	// Create a new server:
	// 		POST /api/v1/servers
	// 			in: `ServerWithVPNConfig` as json.
	// 			out: <empty>.
	ServersURL = "/api/v1/servers"

	// Get statuses of VPN Servers and Healthcheck service:
	// 		GET /api/v1/servers/statuses
	// 			in: <empty>.
	// 			out: `ServiceStatuses` as json.
	ServersStatusesURL = "/api/v1/servers/statuses"

	// Get existing server by its ID:
	// 		GET /api/v1/servers/:id
	// 			in: <empty>.
	// 			out: `Server` as json.
	// Update existing server by its ID:
	// 		PATCH /api/v1/servers/:id
	// 			in: `ServerUpdate` as json.
	// 			out: `Server` as json.
	// Remove existing server by its ID:
	// 		DELETE /api/v1/servers/:id
	// 			in: <empty>.
	// 			out: <empty>.
	ServerURL = "/api/v1/servers/:id"

	// Get VPN configuration for a server:
	// 		GET /api/v1/servers/:id/vpn/config
	// 			in: <empty>.
	// 			out: `ServerWireGuardInterface` as json.
	// Update VPN configuration for a server:
	// 		PATCH /api/v1/servers/:id/vpn/config
	// 			in: `ServerWireGuardInterface` as json.
	// 			out: <empty>.
	ServerVPNConfigURL = "/api/v1/servers/:id/vpn/config"

	// Get list of users:
	// 		GET /api/v1/users
	// 			in: <empty>.
	// 			out: `UserList` as json.
	// Create a new user:
	// 		POST /api/v1/users
	// 			in: `UserWithPassword` as json.
	// 			out: <empty>.
	UsersURL = "/api/v1/users"

	// Get existing user by ID:
	// 		GET /api/v1/users/:id
	// 			in: <empty>.
	// 			out: `User` as json.
	// Update existing user by ID:
	// 		PATCH /api/v1/users/:id
	// 			in: `UserUpdate` as json.
	// 			out: `User` as json.
	// Remove existing user by ID:
	// 		DELETE /api/v1/users/:id
	// 			in: <empty>.
	// 			out: <empty>.
	UserURL = "/api/v1/users/:id"

	// Update user password:
	// 		PATCH /api/v1/users/:id/password
	// 			in: `UserPasswordUpdate` as json.
	// 			out: <empty>.
	UserPasswordChangeURL = "/api/v1/users/:id/password"

	// Get JWT for specified credentials:
	// 		POST api/v1/jwt
	// 			in: `JWTRequestCreds` as json.
	// 			out: `JWT` as json.
	GetJWTURL = "/api/v1/jwt"

	// Get device template for a user:
	// 		GET /api/v1/users/:id/device-template
	// 			in: <empty>.
	// 			out: `DeviceTemplate` as json.
	// Add device template for a user:
	// 		POST /api/v1/users/:id/device-template
	// 			in: `DeviceTemplate` as json.
	// 			out: <empty>.
	// Update device template for a user:
	// 		PATCH /api/v1/users/:id/device-template
	// 			in: `DeviceTemplate` as json.
	// 			out: <empty>.
	// Remove device template for a user:
	// 		DELETE /api/v1/users/:id/device-template
	// 			in: <empty>.
	// 			out: <empty>.
	DeviceTemplateURL = "/api/v1/users/:id/device-template"

	// Get MFA enabled authentication types:
	// 		GET api/v1/mfa/types
	// 			in: <empty>.
	// 			out: `MfaEnabledTypes` as json.
	MFAEnabledAuthTypesURL = "api/v1/mfa/types"

	// Get MFA TOTP secrets for user:
	// 		GET api/v1/users/:id/totp
	// 			in: <empty>.
	// 			out: `MFAUserSecrets` as json.
	// Create MFA TOTP secrets for user:
	// 		POST api/v1/users/:id/totp
	// 			in: <empty>.
	// 			out: <MFAUserSecrets>.
	// Patch user MFA TOTP secrets:
	// 		PATCH api/v1/users/:id/totp
	// 			in: <empty>.
	// 			out: `MFAUserSecrets` as json.
	MFATOTPSecretsURL = "api/v1/users/:id/totp"

	// Get MFA certificate CAs:
	// 		GET api/v1/mfa/cert/ca
	// 			in: <empty>.
	// 			out: `MfaCAList` as json.
	// Add MFA certificate CA with ID:
	// 		POST api/v1/mfa/cert/ca
	// 			in: `Cert`.
	// 			out: <empty>.
	MFACertCAsURL = "api/v1/mfa/cert/ca"

	// Get MFA certificate CRLs:
	// 		GET api/v1/mfa/cert/crl
	// 			in: <empty>.
	// 			out: `MfaCRLList` as json.
	// Add MFA certificate CRL with ID:
	// 		POST api/v1/mfa/cert/crl
	// 			in: `Cert` as json.
	// 			out: <empty>.
	MFACertCRLsURL = "api/v1/mfa/cert/crl"

	// Remove MFA certificate CA with ID:
	// 		DELETE api/v1/mfa/cert/ca/:id
	// 			in: <empty>.
	// 			out: <empty>.
	MFACertCAURL = "api/v1/mfa/cert/ca/:id"

	// Remove MFA certificate CRL with ID:
	// 		DELETE api/v1/mfa/cert/crl/:id
	// 			in: <empty>.
	// 			out: <empty>.
	MFACertCRLURL = "api/v1/mfa/cert/crl/:id"

	// Add VPN adjacency:
	// 		POST /api/v1/vpn/adjacencies
	// 			in:`Adjacency` as json.
	// 			out: <empty>.
	VPNAdjacenciesURL = "/api/v1/vpn/adjacencies"

	// Update VPN adjacency:
	// 		PATCH /api/v1/vpn/adjacencies/:id
	// 			in:`Adjacency` as json.
	// 			out: <empty>.
	// Remove VPN adjacency:
	// 		DELETE /api/v1/vpn/adjacencies/:id
	// 			in: <empty>.
	// 			out: <empty>.
	VPNAdjacencyURL = "/api/v1/vpn/adjacencies/:id"

	// Get list of VPN adjacency templates for a user:
	// 		GET /api/v1/vpn/adjacency-templates/users/:id
	// 			in: <empty>.
	// 			out: `AdjacencyTemplateList` as json.
	VPNUserAdjacencyTemplatesURL = "/api/v1/vpn/adjacency-templates/users/:id"

	// Add VPN adjacency template (user-level adjacency):
	// 		POST /api/v1/vpn/adjacency-templates
	// 			in:`AdjacencyTemplate` as json.
	// 			out: <empty>.
	VPNAdjacencyTemplatesURL = "/api/v1/vpn/adjacency-templates"

	// Update VPN adjacency template:
	// 		PATCH /api/v1/vpn/adjacency-templates/:id
	// 			in:`AdjacencyTemplate` as json.
	// 			out: <empty>.
	// Remove VPN adjacency template:
	// 		DELETE /api/v1/vpn/adjacency-templates/:id
	// 			in: <empty>.
	// 			out: <empty>.
	VPNAdjacencyTemplateURL = "/api/v1/vpn/adjacency-templates/:id"

	// Get list of VPN adjacencies for a server:
	// 		GET /api/v1/vpn/adjacencies/servers/:id
	// 			in: <empty>.
	// 			out: `AdjacencyList` as json.
	VPNServerAdjacenciesURL = "/api/v1/vpn/adjacencies/servers/:id"

	// Get list of VPN adjacencies for devices belonging to a user:
	// 		GET /api/v1/vpn/adjacencies/user/:id
	// 			in: <empty>.
	// 			out: `AdjacencyList` as json.
	VPNClientAdjacenciesURL = "/api/v1/vpn/adjacencies/user/:id"

	// Get secrets for a server:
	// 		POST /api/v1/servers/secrets
	// 			in: `VPNServerSecretsRequest` as json.
	// 			out: `VPNServerSecrets` as json.
	// Making request to this endpoint will invalidate previously returned secrets.
	VPNServerSecretsURL = "/api/v1/servers/secrets"

	// Get list of LDAP configurations:
	// 		GET /api/v1/ldap
	// 			in: <empty>.
	// 			out: `LdapConfigList` as json.
	// Create a new LDAP configuration:
	// 		POST /api/v1/ldap
	// 			in: `LdapConfig` as json.
	// 			out: `LdapConfigGet` as json.
	LdapConfigsURL = "/api/v1/ldap"

	// Get existing LDAP configuration by ID:
	// 		GET /api/v1/ldap/:id
	// 			in: <empty>.
	// 			out: `LdapConfigGet` as json.
	// Remove existing LDAP configuration by ID:
	// 		DELETE /api/v1/ldap/:id
	// 			in: <empty>.
	// 			out: <empty>.
	// Update LDAP configuration:
	// 		PATCH /api/v1/ldap/:id
	// 			in: `LdapConfig` as json.
	// 			out: <empty>.
	LdapConfigURL = "/api/v1/ldap/:id"

	// Get enabled premium features:
	// 		GET /api/v1/features
	// 			in: <empty>.
	// 			out: `FeaturesGet` as json.
	FeaturesURL = "/api/v1/features"

	// Do LDAP resync before resync interval:
	// 		POST /api/v1/ldap/sync/resync
	// 			in: <empty>.
	// 			out: <empty>.
	LdapResyncURL = "/api/v1/ldap/sync/resync"

	// Get LDAP sync status:
	// 		GET api/v1/ldap/sync/status
	// 			in: <empty>.
	// 			out: `LdapSyncStatus` as json.
	LdapSyncStatusURL = "api/v1/ldap/sync/status"

	// Get all LDAP templates:
	// 		GET /api/v1/ldap-templates
	// 			in: <empty>.
	// 			out: `LdapTemplateList` json.
	// Create a new LDAP template:
	// 		POST /api/v1/ldap-templates
	// 			in: `LdapTemplate` as json.
	// 			out: <empty>.
	LdapTemplatesURL = "/api/v1/ldap-templates"

	// Remove existing LDAP template by id:
	// 		DELETE /api/v1/ldap-templates/:id
	// 			in: <empty>.
	// 			out: <empty>.
	// Update LDAP template:
	// 		PATCH /api/v1/ldap-templates/:id
	// 			in: `LdapTemplate` as json.
	// 			out: <empty>.
	LdapTemplateURL = "/api/v1/ldap-templates/:id"

	// Get all LDAP template servers:
	// 		GET /api/v1/ldap-templates/:id/servers
	// 			in: <empty>.
	// 			out: `LdapTemplateServerList` json.
	// Create a new LDAP template server:
	// 		POST /api/v1/ldap-templates/:id/servers
	// 			in: `LdapTemplateServer` as json.
	// 			out: <empty>.
	LdapTemplateServersURL = "/api/v1/ldap-templates/:id/servers"

	// Remove existing LDAP template server:
	// 		DELETE /api/v1/ldap-templates/:id/servers/:server-id
	// 			in: <empty>.
	// 			out: <empty>.
	// Update LDAP template interface:
	//
	// URL parameter `server-id` is ID of the old server.
	// `LdapTemplateServer` contains ID of the new server.
	// 		PATCH /api/v1/ldap-templates/:id/servers/:server-id
	// 			in: `LdapTemplateServer` as json.
	// 			out: <empty>.
	LdapTemplateServerURL = "/api/v1/ldap-templates/:id/servers/:server-id"

	// Get list of address pools:
	// 		GET /api/v1/address-pools
	// 			in: <empty>.
	// 			out: `AddressPoolList` as json.
	// Create a new address pool:
	// 		POST /api/v1/address-pools
	// 			in: `AddressPool` as json.
	// 			out: `AddressPool` as json.
	AddressPoolsURL = "/api/v1/address-pools"

	// Get existing address pool by ID:
	// 		GET /api/v1/address-pools/:id
	// 			in: <empty>.
	// 			out: `AddressPool` as json.
	// Update existing address pool by ID:
	// 		PATCH /api/v1/address-pools/:id
	// 			in: `AddressPool` as json.
	// 			out: `AddressPool` as json.
	// Remove existing address pool by ID:
	// 		DELETE /api/v1/address-pools/:id
	// 			in: <empty>.
	// 			out: <empty>.
	AddressPoolURL = "/api/v1/address-pools/:id"

	// Get list of devices for a device template:
	// 		GET /api/v1/device-templates/:id/devices
	// 			in: <empty>.
	// 			out: `DeviceListResponse` as json.
	// Create a new device:
	// 		POST /api/v1/device-templates/:id/devices
	// 			in: `DeviceData` as json.
	// 			out: <empty>.
	DevicesURL = "/api/v1/device-templates/:id/devices"

	// Get autogenerated device data:
	// 		GET /api/v1/device-templates/:id/devices/autogen
	// 			in: <empty>.
	// 			out: `DeviceData` as json.
	DeviceAutogenURL = "/api/v1/device-templates/:id/devices/autogen"

	// Update device:
	// 		PATCH /api/v1/devices/:id
	// 			in: `DeviceData` as json.
	// 			out: <empty>.
	// Remove device:
	// 		DELETE /api/v1/devices/:id
	// 			in: <empty>.
	// 			out: <empty>.
	DeviceURL = "/api/v1/devices/:id"

	// Resync devices for a device template (reallocate addresses from address pools):
	// 		POST /api/v1/device-templates/:id/devices/resync
	// 			in: <empty>.
	// 			out: `DeviceResyncResponse` as json.
	DeviceTemplateResyncURL = "/api/v1/device-templates/:id/devices/resync"

	// Resync device adjacencies for a user:
	// 		POST /api/v1/users/:id/device-adjacencies/resync
	// 			in: <empty>.
	// 			out: `DeviceAdjacencyResyncResponse` as json.
	UserDeviceAdjacenciesResyncURL = "/api/v1/users/:id/device-adjacencies/resync"
)

// List of available values for the status field of VpnS and Healthcheck
const (
	ServiceStatusGreen   = "GREEN"
	ServiceStatusRed     = "RED"
	ServiceStatusGray    = "GRAY"
	ServiceStatusDefault = "RED"
)

// ErrorResponse defines REST API response in case of error.
type ErrorResponse struct {
	Message string `json:"message"`
}

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username" binding:"required"`
	IsAdmin      bool      `json:"is_admin"`
	MFAType      string    `json:"mfa_type"`
	AuthType     string    `json:"auth_type"`
	Notification string    `json:"notification"`
	UpdatedAt    time.Time `json:"last_edit_time"`
}

type UserWithPassword struct {
	ID           int       `json:"id"`
	Username     string    `json:"username" binding:"required"`
	Password     string    `json:"password" binding:"required"`
	IsAdmin      bool      `json:"is_admin"`
	MFAType      string    `json:"mfa_type"`
	AuthType     string    `json:"auth_type"`
	Notification string    `json:"notification"`
	UpdatedAt    time.Time `json:"last_edit_time"`
}

// UserUpdate defines fields of User which can be updated.
type UserUpdate struct {
	Username     string `json:"username" binding:"required"`
	IsAdmin      bool   `json:"is_admin"`
	MFAType      string `json:"mfa_type"`
	Notification string `json:"notification"`
}

type UserPasswordUpdate struct {
	Password string `json:"password" binding:"required"`
}

type JWTRequestCreds struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type JWT struct {
	Token string `json:"jwt"`
}

type UserList struct {
	Users []User `json:"users"`
}

type MFAUserSecrets struct {
	Secrets json.RawMessage `json:"secrets" binding:"required"`
}

type Server struct {
	ID                 int    `json:"id"`
	Name               string `json:"name" binding:"required"`
	Endpoint           string `json:"endpoint" binding:"required"`
	HealthCheckAddress string `json:"healthcheck_address"`
	// Description has max length of 250 characters.
	Description string `json:"description"`
}

type ServiceStatus struct {
	ID   int    `json:"id"`
	Name string `json:"name" binding:"required"`

	// VPN Server status can be either "GREEN" or "RED". By default status is "RED".
	VPNServer Status `json:"vpn_server"`

	// Healthcheck service status can be "GREEN", "GRAY" or "RED".
	// If server healthcheck address is set then by default status is "RED" otherwise "GRAY".
	Healthcheck Status `json:"healthcheck"`
}

type Status struct {
	Status      string   `json:"status"`
	MessageKey  string   `json:"message_key"`
	MessageArgs []string `json:"message_args"`
}

type ServerWithVPNConfig struct {
	Server
	Config ServerWireGuardInterface `json:"vpn_config" binding:"required"`
}

type ServerUpdate struct {
	Name               string `json:"name" binding:"required"`
	Endpoint           string `json:"endpoint" binding:"required"`
	HealthCheckAddress string `json:"healthcheck_address"`
	// Description has max length of 250 characters.
	Description string `json:"description"`
}

type ServerList struct {
	Servers []Server `json:"servers"`
}

type ServiceStatuses struct {
	Statuses []ServiceStatus `json:"statuses"`
}

// ServerWireGuardInterface defines WireGuard VPN configuration for a server.
type ServerWireGuardInterface struct {
	Name       string   `json:"name"`
	PrivateKey string   `json:"private_key"`
	PublicKey  string   `json:"public_key"`
	Addresses  []string `json:"addresses,omitempty"`
	ListenPort string   `json:"listen_port,omitempty"`
	DNS        []string `json:"dns,omitempty"`
	MTU        string   `json:"mtu,omitempty"`
	// PersistentKeepalive value will be used in a peer section for all VpnCs configuration only if set.
	PersistentKeepalive string `json:"persistent_keepalive,omitempty"`
}

// Adjacency defines input structure for creating adjacency between device and server.
type Adjacency struct {
	ServerID int                 `json:"server_id" binding:"required"`
	DeviceID int                 `json:"device_id" binding:"required"`
	Config   *WireGuardAdjacency `json:"config"`
}

// AdjacencyTemplate defines input structure for creating and updating adjacency template between user and server.
type AdjacencyTemplate struct {
	ServerID       int                         `json:"server_id" binding:"required"`
	UserID         int                         `json:"user_id" binding:"required"`
	TemplateConfig *WireGuardAdjacencyTemplate `json:"template_config"`
}

type AdjacencyExtended struct {
	ID     int                  `json:"id"`
	Server *Server              `json:"server,omitempty"`
	Device *Device              `json:"device,omitempty"`
	Config *WireGuardPeerConfig `json:"config"`
}

type AdjacencyTemplateExtended struct {
	ID             int                         `json:"id"`
	Server         *Server                     `json:"server,omitempty"`
	Device         *Device                     `json:"device,omitempty"`
	TemplateConfig *WireGuardAdjacencyTemplate `json:"template_config"`
}

type AdjacencyList struct {
	Adjacencies []AdjacencyExtended `json:"adjacencies"`
}

type AdjacencyTemplateList struct {
	AdjacencyTemplates []AdjacencyTemplateExtended `json:"adjacency_templates"`
}

type DeviceTemplate struct {
	ID            int            `json:"id"`
	InterfaceName string         `json:"interface_name"`
	AddressPools  []*AddressPool `json:"address_pools"`
	ListenPort    string         `json:"listen_port,omitempty"`
	DNS           []string       `json:"dns,omitempty"`
	MTU           string         `json:"mtu,omitempty"`
}

type WireGuardAdjacency struct {
	// PresharedKey will be used in a peer section of both VpnC and VpnS if set.
	PresharedKey string `json:"preshared_key,omitempty"`
	// If ServerSide is true, AllowedIPs will be used in a peer section of VpnS configuration,
	// otherwise it will be used in a peer section of VpnC configuration (required).
	AllowedIPs []string `json:"allowed_ips"`
	// If ServerSide is true, OtherSideAllowedIPs will be used in a peer section of VpnC configuration,
	// otherwise it is unused (required).
	OtherSideAllowedIPs []string `json:"other_side_allowed_ips"`
	// ServerSide controls usage of AllowedIPs and OtherSideAllowedIPs. (required)
	ServerSide bool `json:"server_side"`
}

type WireGuardAdjacencyTemplate struct {
	// PresharedKey will be used in a peer section of both VpnC and VpnS if set.
	UsePresharedKey bool `json:"use_preshared_key"`
	// ClientSideAllowedIPs will be used in a peer section of VpnC configuration (required).
	ClientSideAllowedIPs []string `json:"client_side_allowed_ips"`
}

type WireGuardPeer struct {
	ID       int                  `json:"id,omitempty"`
	Server   *Server              `json:"server,omitempty"`
	DeviceID int                  `json:"device_id,omitempty"`
	Config   *WireGuardPeerConfig `json:"peer_config"`
}

type WireGuardPeerConfig struct {
	PublicKey           string   `json:"public_key"`
	PresharedKey        string   `json:"preshared_key,omitempty"`
	AllowedIPs          []string `json:"allowed_ips,omitempty"`
	OtherSideAllowedIPs []string `json:"other_side_allowed_ips,omitempty"`
	Endpoint            string   `json:"endpoint,omitempty"`
	PersistentKeepalive string   `json:"persistent_keepalive,omitempty"`
}

type VPNServerSecretsRequest struct {
	ServerName string `json:"server_name" binding:"required"`
}

type LdapConfig struct {
	Priority       int    `json:"priority"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	BaseDN         string `json:"base_dn"`
	BindDN         string `json:"bind_dn"`
	BindPW         string `json:"bind_pw"`
	UserListFilter string `json:"user_list_filter"`
	UsernameAttr   string `json:"username_attribute"`
	UIDAttr        string `json:"uid_attribute"`
	UseTLS         bool   `json:"use_tls"`
	FQDN           string `json:"fqdn,omitempty"`
	CACert         string `json:"ca_cert,omitempty"`
	TemplateID     int    `json:"template_id,omitempty"`
}

type LdapTemplateServer struct {
	ServerID             int      `json:"server_id" binding:"required"`
	ServerName           string   `json:"server_name"`
	ClientSideAllowedIPs []string `json:"client_side_allowed_ips"`
	UsePresharedKey      bool     `json:"use_preshared_key"`
}

type LdapTemplate struct {
	ID            int                  `json:"id"`
	Name          string               `json:"name" binding:"required"`
	Filter        string               `json:"filter"`
	IsAdmin       bool                 `json:"is_admin"`
	MFAType       string               `json:"mfa_type"`
	Servers       []LdapTemplateServer `json:"servers,omitempty"`
	InterfaceName string               `json:"iface_name" binding:"required"`
	ListenPort    string               `json:"listen_port,omitempty"`
	MTU           string               `json:"mtu,omitempty"`
	DNS           []string             `json:"dns,omitempty"`
	AddressPools  []*AddressPool       `json:"address_pools"`
}

type LdapConfigGet struct {
	ID             string `json:"id"`
	Priority       int    `json:"priority"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	BaseDN         string `json:"base_dn"`
	BindDN         string `json:"bind_dn"`
	UserListFilter string `json:"user_list_filter"`
	UsernameAttr   string `json:"username_attribute"`
	UIDAttr        string `json:"uid_attribute"`
	UseTLS         bool   `json:"use_tls"`
	FQDN           string `json:"fqdn,omitempty"`
	CACert         string `json:"ca_cert,omitempty"`
	TemplateID     int    `json:"template_id"`
}

type LdapConfigList struct {
	LdapConfigs []*LdapConfigGet `json:"ldap_configs"`
}

type AllLdapTemplatesResponse struct {
	LdapTemplates []*LdapTemplate `json:"ldap_templates"`
}

type LdapTemplateServerList struct {
	LdapTemplateServers []*LdapTemplateServer `json:"ldap_template_servers"`
}

type FeaturesGet struct {
	LDAP bool `json:"ldap"`
}

type LdapSyncStatus struct {
	StartedAt             time.Time `json:"started_at"`
	InProgress            bool      `json:"in_progress"`
	SyncDuration          string    `json:"sync_duration"`
	LDAPServersConfigured int       `json:"ldap_servers_configured"`
	LDAPServersProcessed  int       `json:"ldap_servers_processed"`
	UsersRetrieved        int       `json:"users_retrieved"`
	UsersCreated          int       `json:"users_created"`
	UsersUpdated          int       `json:"users_updated"`
	UsersDeleted          int       `json:"users_deleted"`
	RelationsCreated      int       `json:"relations_created"`
	RelationsUpdated      int       `json:"relations_updated"`
	RelationsDeleted      int       `json:"relations_deleted"`
	ErrorCount            int       `json:"error_count"`
	Errors                []string  `json:"error_list"`
}

// Used both for CAs and CRLs data
type Cert struct {
	Data string `json:"data"`
}

type MfaCAList struct {
	CAs []MfaCA `json:"ca_list"`
}

type MfaCA struct {
	ID        int    `json:"id"`
	CN        string `json:"common_name"`
	NotBefore string `json:"not_before"`
	NotAfter  string `json:"not_after"`
}

type MfaCRLList struct {
	CRLs []MfaCRL `json:"crl_list"`
}

type MfaCRL struct {
	ID                  int      `json:"id"`
	Issuer              string   `json:"issuer"`
	RevokedCertificates []string `json:"serial_numbers"`
}

type MfaEnabledTypes struct {
	Types []string `json:"types"`
}

type AddressPool struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StartAddr   string `json:"start_addr"`
	EndAddr     string `json:"end_addr"`
	NetMask     int    `json:"net_mask"`
}

type AddressPoolListResponse struct {
	AddressPools []*AddressPool `json:"address_pools"`
}

type Device struct {
	ID                int `json:"id"`
	DeviceData        `json:",inline"`
	ExternalDeviceID  string    `json:"device_id"`
	SessionID         string    `json:"session_id"`
	LastTimeConnected time.Time `json:"last_time_connected"`
	DeviceInformation string    `json:"device_information"`
}

type DeviceData struct {
	Description string   `json:"description"`
	PrivateKey  string   `json:"private_key"`
	PublicKey   string   `json:"public_key"`
	Addresses   []string `json:"addresses"`
}

type DeviceListResponse struct {
	Devices []*Device `json:"devices"`
}

type DeviceResyncResponse struct {
	DevicesUpdated int      `json:"devices_updated"`
	DevicesDeleted int      `json:"devices_deleted"`
	Errors         []string `json:"errors"`
}

type DeviceAdjacencyResyncResponse struct {
	AdjacenciesCreated int      `json:"adjacencies_created"`
	AdjacenciesUpdated int      `json:"adjacencies_updated"`
	AdjacenciesDeleted int      `json:"adjacencies_deleted"`
	Errors             []string `json:"errors"`
}
