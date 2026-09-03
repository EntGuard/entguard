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

package vcm

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	log "github.com/sirupsen/logrus"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/addresspools"
	"github.com/entguard/entguard/pkg/certs"
	"github.com/entguard/entguard/pkg/crypto/aesgcm"
	"github.com/entguard/entguard/pkg/crypto/keys"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/ip"
	"github.com/entguard/entguard/pkg/rand"
	pbapiv1 "github.com/entguard/entguard/proto/v1"
	pbapiv2 "github.com/entguard/entguard/proto/v2"
	"github.com/entguard/entguard/service/api/v1"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	Organization     = "PANTHEON.tech"
	OrganizationUnit = "cnf-vpn-o"

	tagLength                           = 16
	postgresReportCaller                = "VCM (postgres)"
	orchestratorNonKubernetesReplicaNum = 1
)

type postgresBasedVCM struct {
	pg    *pg.VPNConfigQuerier
	sqlcQ *sqlc.Queries
	cp    *certs.Provider

	// rwMux protects map of server IDs to config watchers.
	rwMux                  sync.RWMutex
	configWatchers         map[string]configWatcher // key is server tag, that is unique server instance ID within one server config(serverID) (Note: server config can be applied to multiple server instances)
	sessionsUpdatesWatcher chan *pbapiv2.SubscribeForSessionsUpdatesResponse
	kubernetesClientset    *kubernetes.Clientset
	logger                 *log.Entry

	encryptionKey []byte
}

type configWatcher struct {
	updates chan<- *pbapiv1.WireGuardNotification
}

func getKubernetesClientset() (*kubernetes.Clientset, error) {
	kubernetesConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	kubernetesClientset, err := kubernetes.NewForConfig(kubernetesConfig)
	if err != nil {
		return nil, err
	}

	return kubernetesClientset, nil
}

// NewVCMPostgresBased returns new VPN configuration manager based on PostgreSQL storage.
func NewVCMPostgresBased(ctx context.Context, conn *pgxpool.Pool, l *log.Logger, reinitCert bool, encryptionKey []byte) (VPNConfigManager, error) {
	kubernetesClientset, err := getKubernetesClientset()
	if err != nil {
		l.Debugf("unable to get kubernetes clientset, assuming basic non-k8s deployment: %v", err)
	}

	m := &postgresBasedVCM{
		pg:                  pg.NewQuerier(ctx, conn, l),
		sqlcQ:               sqlc.New(conn),
		configWatchers:      make(map[string]configWatcher),
		kubernetesClientset: kubernetesClientset,
		logger:              l.WithField("reportCaller", postgresReportCaller),
		encryptionKey:       encryptionKey,
	}

	cp, err := m.InitCertificateProvider(ctx, reinitCert)
	if err != nil {
		return nil, err
	}
	m.cp = cp

	return m, nil
}

func (m *postgresBasedVCM) SetKubernetesClientset(ctx context.Context, clients *kubernetes.Clientset) {
	m.kubernetesClientset = clients
}

// InitCertificateProvider will create a new CertificateProvider or will initialize it from the DB data.
func (m *postgresBasedVCM) InitCertificateProvider(ctx context.Context, reinit bool) (*certs.Provider, error) {
	if reinit {
		m.logger.Info("Removing old root CA from the DB")
		err := m.sqlcQ.DeleteCA(ctx)
		if err != nil {
			return nil, m.handleError(err)
		}
		err = m.pg.DeleteUsedTags(ctx)
		if err != nil {
			return nil, m.handleError(err)
		}
	}

	var cp *certs.Provider
	existingCA, err := m.sqlcQ.GetCA(ctx)

	if err != nil {
		if err != pgx.ErrNoRows {
			return nil, m.handleError(err)
		}

		m.logger.Info("Initializing a new certificate provider")

		cp, err = certs.NewProvider(Organization, OrganizationUnit)
		if err != nil {
			return nil, err
		}

		key, err := x509.MarshalPKCS8PrivateKey(cp.CAKey)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Root CA private key: %v", err)
		}
		keyEnc, err := aesgcm.Seal(m.encryptionKey, key)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt Root CA private key: %v", err)
		}

		_, err = m.sqlcQ.InsertCA(ctx, sqlc.InsertCAParams{
			Cert:         cp.CACert.Raw,
			KeyEncrypted: keyEnc,
		})
		if err != nil {
			return nil, m.handleError(err)
		}
	} else {
		m.logger.Info("Initializing certificate provider from DB data")

		parsedCert, err := x509.ParseCertificate(existingCA.Cert)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Root CA certificate: %v", err)
		}

		key, err := aesgcm.OpenBytes(m.encryptionKey, existingCA.KeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt Root CA private key: %v", err)
		}
		parsedKey, err := x509.ParsePKCS8PrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Root CA private key: %v", err)
		}

		cp = &certs.Provider{
			CACert: parsedCert,
			CAKey:  parsedKey,
			Orgz:   Organization,
		}
	}
	return cp, nil
}

type PeerNotification struct {
	OldServerId        int
	NewServerId        int
	AllowedIps         []string
	AdjacencyId        int
	OldDevicePublicKey string
	NewDevicePublicKey string
}

// HandlePeerUpdates listens for notifications about changes made to the 'adjacencies' table and forwards them to concerned EntGuard server instances.
func (m *postgresBasedVCM) HandlePeerUpdates(ctx context.Context, fails chan error) {
	notificationChannel := m.pg.ListenForWGPeerUpdates(ctx, fails)

	for rawNotification := range notificationChannel {
		var notification PeerNotification
		err := json.Unmarshal([]byte(rawNotification), &notification)
		if err != nil {
			fails <- fmt.Errorf("unable to unmarshal 'adjacencies' table notification: %v", err)
			continue
		}

		tagsOld, err := m.pg.GetServerTags(ctx, notification.OldServerId)
		if err != nil {
			fails <- fmt.Errorf("can't get server tags that belong to server ID %d: %v", notification.OldServerId, err)
		}
		tagsNew, err := m.pg.GetServerTags(ctx, notification.NewServerId)
		if err != nil {
			fails <- fmt.Errorf("can't get server tags that belong to server ID %d: %v", notification.NewServerId, err)
		}

		m.applyForWatchedTags(tagsOld, notification.OldServerId, "Sending delete peer notification to server", func(tag string) {
			m.NotifyWGDelPeerPubKey(tag, notification.OldDevicePublicKey)
		})

		// Note: Sending preshared key directly in notification payload is security issue. Hence notification is
		// sending adjacency id that needs to be resolved by making additional query to DB
		presharedKey, err := m.resolvePresharedKey(ctx, notification.AdjacencyId)
		if err != nil {
			fails <- fmt.Errorf("can't resolve preshared key from peer updates notification: %v", err)
		}

		m.applyForWatchedTags(tagsNew, notification.NewServerId, "Sending add peer notification to server", func(tag string) {
			m.NotifyWGAddPeer(tag, notification.NewDevicePublicKey, presharedKey, notification.AllowedIps)
		})
	}
}

// resolvePresharedKey resolves preshared key value from adjacencyId by making call to DB to retrieve
// the given adjacency where the needed preshared key is stored. In special case of bad adjacency id (includes
// zero-valued  adjacency id for notification about removed adjacency), the empty preshared key is returned.
// In error case, the preshared key is properly filled too (with empty value as unresolved value) together with
// returned non-nil error.
func (m *postgresBasedVCM) resolvePresharedKey(ctx context.Context, adjacencyId int) (string, error) {
	const unresolvedPresharedKey = ""
	if adjacencyId <= 0 {
		return unresolvedPresharedKey, nil
	}

	adjacency, err := m.sqlcQ.GetAdjacency(ctx, adjacencyId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return unresolvedPresharedKey, fmt.Errorf("there is no adjacency for id %d provided by peer "+
				"updates listener notification, adjacency query error: %v", adjacencyId, err)
		}
		return unresolvedPresharedKey, fmt.Errorf("can't retrieve adjacency for id %d provided by peer "+
			"updates listener notification, adjacency query error: %v", adjacencyId, m.handleError(err))
	}

	presharedKey, err := aesgcm.OpenString(m.encryptionKey, adjacency.PresharedKeyEncrypted)
	if err != nil {
		return unresolvedPresharedKey, fmt.Errorf("can't decrypt preshared key for adjacency with id %d: %v",
			adjacencyId, m.handleError(err))
	}
	return presharedKey, nil
}

func (m *postgresBasedVCM) applyForWatchedTags(tags []string, serverID int, logMessage string, applyFn func(tag string)) {
	for _, tag := range tags {
		m.rwMux.Lock()
		_, found := m.configWatchers[tag]
		m.rwMux.Unlock()
		if found {
			m.logger.
				WithField("serverID", serverID).
				WithField("tag", tag).
				Info(logMessage)
			applyFn(tag)
		}
	}
}

type ServerConfigNotification struct {
	ServerID int
}

// HandleServerUpdates listens for notifications about changes made to the 'server_wg_configs' table(and after related
// wg_iface_addresses changes) and forwards them to concerned EntGuard server instances.
func (m *postgresBasedVCM) HandleServerUpdates(ctx context.Context, fails chan error) {
	notificationChannel := m.pg.ListenForWGConfigsUpdates(ctx, fails)

	for rawNotification := range notificationChannel {
		var notification ServerConfigNotification
		err := json.Unmarshal([]byte(rawNotification), &notification)
		if err != nil {
			fails <- fmt.Errorf("unable to unmarshal 'wg_serverconfigs' table notification: %v", err)
			continue
		}

		// get complete server configuration (config + adjacencies)
		m.logger.
			WithField("serverID", notification.ServerID).
			Infof("Retrieving data for server config change notification that will be send to server")
		cfg, err := m.GetWireGuardServerConfig(ctx, notification.ServerID)
		if err != nil {
			fails <- fmt.Errorf("can't get server config (for server notification) that belong "+
				"to server ID %d: %v", notification.ServerID, err)
		}
		adjacencies, err := m.GetWireGuardServerAdjacencies(ctx, notification.ServerID)
		if err != nil {
			fails <- fmt.Errorf("can't get server adjacencies (for server notification) that belong "+
				"to server ID %d: %v", notification.ServerID, err)
		}
		wgConfig, err := wireguardConfigToProto(cfg, adjacencies)
		if err != nil {
			fails <- fmt.Errorf("can't process server config and adjacencies (for server notification) "+
				"that belong to server ID %d: %v", notification.ServerID, err)
		}

		// send notification further to all server instances (identified by unique tag within one server config(serverID))
		tags, err := m.pg.GetServerTags(ctx, notification.ServerID)
		if err != nil {
			fails <- fmt.Errorf("can't get server tags that belong to server ID %d: %v", notification.ServerID, err)
		}
		m.applyForWatchedTags(tags, notification.ServerID, "Sending server config change notification to server", func(tag string) {
			m.notifyWGServerConfigChange(tag, wgConfig)
		})
	}
}

type DeviceSessionNotification struct {
	DeviceID   int
	OldSession string
	NewSession string
}

// HandleSessionsUpdatesForHealthcheckService listens for notifications about changes made to the 'devices' table 'session_id' column and forwards them healthcheck service.
func (m *postgresBasedVCM) HandleSessionsUpdatesForHealthcheckService(ctx context.Context, fails chan error) {
	notificationChannel := m.pg.ListenForDevicesSessionsUpdates(ctx, fails)

	for rawNotification := range notificationChannel {
		var notification DeviceSessionNotification
		err := json.Unmarshal([]byte(rawNotification), &notification)
		if err != nil {
			fails <- fmt.Errorf("unable to unmarshal 'devices' table 'session_id' column into notification: %v", err)
			continue
		}

		enabled, err := m.sqlcQ.IsHealthCheckEnabledForDevice(ctx, notification.DeviceID)
		if err != nil {
			fails <- fmt.Errorf("%w: IsHealthCheckEnabledForDevice: %w", pg.ErrQueryFailed, err)
			continue
		}

		if !enabled {
			continue
		}

		if notification.OldSession != "" {
			m.logger.
				WithField("sessionID", notification.OldSession).
				WithField("deviceID", notification.DeviceID).
				Debug("Sending delete session notification to healthcheck service")
			m.notifyHealthcheckDeleteSession(notification.OldSession)
		}

		if notification.NewSession != "" {
			m.logger.
				WithField("sessionID", notification.NewSession).
				WithField("deviceID", notification.DeviceID).
				Debug("Sending add new session notification to healthcheck service")
			m.notifyHealthcheckAddSession(notification.NewSession)
		}
	}
}

// isServerRunning is a helper function that checks if all replicas of the given server deployment are currently running.
func (m *postgresBasedVCM) AllReplicasRunning(ctx context.Context, server *api.Server) (bool, error) {
	currentReplicasNum, err := m.pg.GetReplicaCount(ctx, server.ID)
	if err != nil {
		return false, fmt.Errorf("GetReplicaCount for serverID=%d: %w", server.ID, err)
	}

	// The Orchestrator is not running within a Kubernetes deployment if kubernetesClientset is nil.
	if m.kubernetesClientset == nil {
		return currentReplicasNum == orchestratorNonKubernetesReplicaNum, nil
	}

	serverDeployment, err := m.kubernetesClientset.AppsV1().Deployments(apiv1.NamespaceDefault).Get(context.Background(), server.Name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("get kubernetesClientset deployments for serverID=%d: %w", server.ID, err)
	}

	desiredReplicasNum := int(*serverDeployment.Spec.Replicas)

	return desiredReplicasNum == currentReplicasNum, nil
}

func (m *postgresBasedVCM) GetAllServers(ctx context.Context) (*api.ServerList, error) {
	servers, err := m.pg.GetAllServers(ctx)
	if err != nil {
		return nil, m.handleError(err)
	}

	sl := &api.ServerList{
		Servers: []api.Server{},
	}
	for _, s := range servers {
		apiServer := s.ToAPI()
		sl.Servers = append(sl.Servers, apiServer)
	}

	return sl, nil
}

func (m *postgresBasedVCM) GetServerByName(ctx context.Context, name string) (*api.Server, error) {
	s, err := m.pg.GetServerByName(ctx, name)
	if err != nil {
		return nil, m.handleError(err)
	}

	apiServer := s.ToAPI()
	return &apiServer, nil
}

func (m *postgresBasedVCM) GetServerByID(ctx context.Context, id int) (*api.Server, error) {
	s, err := m.pg.GetServerByID(ctx, id)
	if err != nil {
		return nil, m.handleError(err)
	}

	apiServer := s.ToAPI()
	return &apiServer, nil
}

func (m *postgresBasedVCM) CheckServerExists(ctx context.Context, id int) (bool, error) {
	exists, err := m.pg.CheckServerIDExists(ctx, id)
	if err != nil {
		return false, m.handleError(err)
	}

	return exists, nil
}

func (m *postgresBasedVCM) CreateServer(ctx context.Context, apiServerCreate *api.ServerWithVPNConfig) error {
	s, err := pg.NewServerFromAPI(apiServerCreate)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidData, err)
	}

	exists, err := m.pg.CheckServerNameExists(ctx, s.Name)
	if err != nil {
		return m.handleError(err)
	}
	if exists {
		return ErrAlreadyExists
	}

	wgc, err := pg.NewWireGuardConfigFromAPI(&apiServerCreate.Config)
	if err != nil {
		return fmt.Errorf("%w : %v", ErrInvalidData, err)
	}

	if _, err := InsertServerWithWireGuardConfig(ctx, m.pg.Conn, m.encryptionKey, s, wgc); err != nil {
		return m.handleError(err)
	}

	m.logger.
		WithField("name", s.Name).
		Info("A new server added")
	return nil
}

func (m *postgresBasedVCM) UpdateServer(ctx context.Context, id int, apiServerUpdate *api.ServerUpdate) (*api.Server, error) {
	s, err := m.pg.GetServerByID(ctx, id)
	if err != nil {
		return nil, m.handleError(err)
	}

	sOld := *s

	err = s.SetName(apiServerUpdate.Name)
	if err != nil {
		return nil, err
	}
	err = s.SetEndpoint(apiServerUpdate.Endpoint)
	if err != nil {
		return nil, err
	}

	err = s.SetHealthCheckAddress(apiServerUpdate.HealthCheckAddress)
	if err != nil {
		return nil, err
	}
	err = s.SetDescription(apiServerUpdate.Description)
	if err != nil {
		return nil, err
	}

	err = m.pg.UpdateServer(ctx, s)
	if err != nil {
		return nil, m.handleError(err)
	}

	if sOld.HealthCheckAddress != nil && s.HealthCheckAddress == nil {
		// healthcheck was removed
		if err := m.removeHealthcheckFromExistingAdjacencies(ctx, id, sOld.HealthCheckAddress); err != nil {
			m.logger.WithError(err).Error("Remove healthcheck from existing adjacencies")
		}
	}
	if s.HealthCheckAddress != nil &&
		(sOld.HealthCheckAddress == nil || s.HealthCheckAddress.Compare(*sOld.HealthCheckAddress) != 0) {
		// if changed, remove the old one first
		if sOld.HealthCheckAddress != nil && s.HealthCheckAddress.Compare(*sOld.HealthCheckAddress) != 0 {
			if err := m.removeHealthcheckFromExistingAdjacencies(ctx, id, sOld.HealthCheckAddress); err != nil {
				m.logger.WithError(err).Error("Remove old healthcheck from existing adjacencies before adding new one")
			}
		}
		// add the new one
		if err := m.updateExistingAdjacenciesWithHealthcheck(ctx, id, s.HealthCheckAddress); err != nil {
			m.logger.WithError(err).Error("Update existing adjacencies with healthcheck address")
		}
	}

	err = m.DeleteInvalidSessionsByServer(ctx, &sOld, s)
	if err != nil {
		return nil, m.handleError(err)
	}

	updatedApiServer := s.ToAPI()
	return &updatedApiServer, nil
}

// updateExistingAdjacenciesWithHealthcheck updates all adjacencies, adjacency templates, and LDAP template servers
// for a given server to include the healthcheck address in their client-side allowed IPs.
func (m *postgresBasedVCM) updateExistingAdjacenciesWithHealthcheck(ctx context.Context, serverID int, healthcheckAddr *netip.Addr) error {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	// update device adjacencies
	adjacencies, err := m.sqlcQ.GetAdjacenciesByServerID(queryCtx, serverID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return fmt.Errorf("GetAdjacenciesByServerID: %w", err)
		}
		// no adjacencies exist for this server yet
	}

	for _, adj := range adjacencies {
		updatedAllowedIPs, modified := EnsureHealthcheckAddrInAllowedIPs(adj.ClientSideAllowedIps, healthcheckAddr)
		if modified {
			if err := m.sqlcQ.UpdateAdjacencyClientSideAllowedIPs(queryCtx, sqlc.UpdateAdjacencyClientSideAllowedIPsParams{
				ID:                   adj.ID,
				ClientSideAllowedIps: updatedAllowedIPs,
			}); err != nil {
				return fmt.Errorf("UpdateAdjacencyClientSideAllowedIPs for adjacency %d: %w", adj.ID, err)
			}
		}
	}

	// update adjacency templates
	adjTemplates, err := m.sqlcQ.GetAdjacencyTemplatesByServerID(queryCtx, serverID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return fmt.Errorf("GetAdjacencyTemplatesByServerID: %w", err)
		}
		// no adjacency templates exist for this server yet
	}

	for _, adjTemplate := range adjTemplates {
		updatedAllowedIPs, modified := EnsureHealthcheckAddrInAllowedIPs(adjTemplate.ClientSideAllowedIps, healthcheckAddr)
		if modified {
			if err := m.sqlcQ.UpdateAdjacencyTemplateClientSideAllowedIPs(queryCtx, sqlc.UpdateAdjacencyTemplateClientSideAllowedIPsParams{
				ID:                   adjTemplate.ID,
				ClientSideAllowedIps: updatedAllowedIPs,
			}); err != nil {
				return fmt.Errorf("UpdateAdjacencyTemplateClientSideAllowedIPs for template %d: %w", adjTemplate.ID, err)
			}
		}
	}

	// update LDAP template servers
	ldapTemplateServers, err := m.sqlcQ.GetLdapTemplateServersByServerID(queryCtx, &serverID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return fmt.Errorf("GetLdapTemplateServersByServerID: %w", err)
		}
		// no LDAP template servers exist for this server yet
	}

	for _, ldapSrv := range ldapTemplateServers {
		updatedAllowedIPs, modified := EnsureHealthcheckAddrInAllowedIPs(ldapSrv.AllowedIps, healthcheckAddr)
		if modified {
			if err := m.sqlcQ.UpdateLdapTemplateServerAllowedIPs(queryCtx, sqlc.UpdateLdapTemplateServerAllowedIPsParams{
				LdapTemplateID: ldapSrv.LdapTemplateID,
				ServerID:       ldapSrv.ServerID,
				AllowedIps:     updatedAllowedIPs,
			}); err != nil {
				return fmt.Errorf("UpdateLdapTemplateServerAllowedIPs for LDAP template %d, server %d: %w", ldapSrv.LdapTemplateID, ldapSrv.ServerID, err)
			}
		}
	}

	return nil
}

// removeHealthcheckFromExistingAdjacencies removes a healthcheck address from all adjacencies associated with a server.
func (m *postgresBasedVCM) removeHealthcheckFromExistingAdjacencies(ctx context.Context, serverID int, healthcheckAddr *netip.Addr) error {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	// remove from device adjacencies
	adjacencies, err := m.sqlcQ.GetAdjacenciesByServerID(queryCtx, serverID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return fmt.Errorf("GetAdjacenciesByServerID: %w", err)
		}
		// no adjacencies exist for this server yet
	}

	for _, adj := range adjacencies {
		updatedAllowedIPs, modified := RemoveHealthcheckAddrFromAllowedIPs(adj.ClientSideAllowedIps, healthcheckAddr)
		if modified {
			if err := m.sqlcQ.UpdateAdjacencyClientSideAllowedIPs(queryCtx, sqlc.UpdateAdjacencyClientSideAllowedIPsParams{
				ID:                   adj.ID,
				ClientSideAllowedIps: updatedAllowedIPs,
			}); err != nil {
				return fmt.Errorf("UpdateAdjacencyClientSideAllowedIPs for adjacency %d: %w", adj.ID, err)
			}
		}
	}

	// remove from adjacency templates
	adjTemplates, err := m.sqlcQ.GetAdjacencyTemplatesByServerID(queryCtx, serverID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return fmt.Errorf("GetAdjacencyTemplatesByServerID: %w", err)
		}
		// no adjacency templates exist for this server yet
	}

	for _, adjTemplate := range adjTemplates {
		updatedAllowedIPs, modified := RemoveHealthcheckAddrFromAllowedIPs(adjTemplate.ClientSideAllowedIps, healthcheckAddr)
		if modified {
			if err := m.sqlcQ.UpdateAdjacencyTemplateClientSideAllowedIPs(queryCtx, sqlc.UpdateAdjacencyTemplateClientSideAllowedIPsParams{
				ID:                   adjTemplate.ID,
				ClientSideAllowedIps: updatedAllowedIPs,
			}); err != nil {
				return fmt.Errorf("UpdateAdjacencyTemplateClientSideAllowedIPs for template %d: %w", adjTemplate.ID, err)
			}
		}
	}

	// remove from LDAP template servers
	ldapTemplateServers, err := m.sqlcQ.GetLdapTemplateServersByServerID(queryCtx, &serverID)
	if err != nil {
		if err != pgx.ErrNoRows {
			return fmt.Errorf("GetLdapTemplateServersByServerID: %w", err)
		}
		// no LDAP template servers exist for this server yet
	}

	for _, ldapSrv := range ldapTemplateServers {
		updatedAllowedIPs, modified := RemoveHealthcheckAddrFromAllowedIPs(ldapSrv.AllowedIps, healthcheckAddr)
		if modified {
			if err := m.sqlcQ.UpdateLdapTemplateServerAllowedIPs(queryCtx, sqlc.UpdateLdapTemplateServerAllowedIPsParams{
				LdapTemplateID: ldapSrv.LdapTemplateID,
				ServerID:       ldapSrv.ServerID,
				AllowedIps:     updatedAllowedIPs,
			}); err != nil {
				return fmt.Errorf("UpdateLdapTemplateServerAllowedIPs for LDAP template server %d: %w", ldapSrv.LdapTemplateID, err)
			}
		}
	}

	return nil
}

func (m *postgresBasedVCM) DeleteServer(ctx context.Context, id int) error {
	exists, err := m.pg.CheckServerIDExists(ctx, id)
	if err != nil {
		return m.handleError(err)
	}
	if !exists {
		return ErrNotFound
	}
	if err := m.pg.DeleteServer(ctx, id); err != nil {
		return m.handleError(err)
	}
	return nil
}

func (m *postgresBasedVCM) GetCACertificate(ctx context.Context) *x509.Certificate {
	return m.cp.CACert
}

func (m *postgresBasedVCM) GetCertForServer(ctx context.Context, serverName string) (cert []byte, key []byte, e error) {
	s, err := m.pg.GetServerByName(ctx, serverName)
	if err != nil {
		return nil, nil, m.handleError(err)
	}
	serverTags, err := m.pg.GetServerTags(ctx, s.ID)
	if err != nil {
		return nil, nil, m.handleError(err)
	}
	newTag := rand.Generate(serverTags, tagLength)
	c, k, err := m.cp.MakeClientCert(serverName, newTag)
	if err != nil {
		return nil, nil, err
	}
	err = m.pg.InsertUsedTag(ctx, newTag, s.ID)
	if err != nil {
		return nil, nil, err
	}
	return c, k, nil
}

func (m *postgresBasedVCM) VerifyServerTag(ctx context.Context, unverifiedTag, serverName string) bool {
	tags, err := m.pg.GetServerTagsByName(ctx, serverName)

	if err != nil {
		return false
	}

	for _, tag := range tags {
		if tag == unverifiedTag {
			return true
		}
	}

	return false
}

func (m *postgresBasedVCM) CheckDeviceTemplateExistsByUserID(ctx context.Context, userID int) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	// TODO: should be deleted after deviceTemplateID is passed directly
	dt, err := m.sqlcQ.GetDeviceTemplateByUserID(queryCtx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("%w: GetDeviceTemplateByUserID: %w", pg.ErrQueryFailed, err)
	}

	r, err := m.sqlcQ.DeviceTemplateExists(queryCtx, dt.ID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, m.handleError(fmt.Errorf("%w: %v", pg.ErrQueryFailed, err))
	}
	return r > 0, nil
}

func (m *postgresBasedVCM) GetDeviceTemplateByUserID(ctx context.Context, userID int) (*api.DeviceTemplate, error) {
	dt, err := getDeviceTemplateByUserID(ctx, m.sqlcQ, userID)
	if err != nil {
		return nil, m.handleError(err)
	}
	apiDT := dt.ToAPI()
	return apiDT, nil
}

func (m *postgresBasedVCM) GetDeviceWireGuardConfig(ctx context.Context, deviceID int) (*api.ServerWireGuardInterface, error) {
	wgc, err := getDeviceWireGuardConfig(ctx, m.sqlcQ, m.encryptionKey, deviceID)
	if err != nil {
		return nil, m.handleError(err)
	}
	apiWGC := wgc.ToAPI()
	return apiWGC, nil
}

func (m *postgresBasedVCM) GetWireGuardServerConfig(ctx context.Context, serverID int) (*api.ServerWireGuardInterface, error) {
	wgc, err := GetWireGuardServerConfig(ctx, m.sqlcQ, m.encryptionKey, serverID)
	if err != nil {
		return nil, m.handleError(err)
	}
	apiWGC := wgc.ToAPI()
	return apiWGC, nil
}

func (m *postgresBasedVCM) CreateDeviceTemplate(ctx context.Context, userID int, apiDT *api.DeviceTemplate) (int, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	dt, err := pg.NewDeviceTemplateFromAPI(apiDT)
	if err != nil {
		return 0, fmt.Errorf("%w : %v", ErrInvalidData, err)
	}

	pools := make([]sqlc.AddressPool, 0, len(dt.AddressPools))
	for _, p := range dt.AddressPools {
		pool, err := m.sqlcQ.GetAddressPoolByID(queryCtx, p.ID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return 0, fmt.Errorf("%w: address pool with ID %d does not exist", ErrInvalidData, p.ID)
			}
			return 0, fmt.Errorf("%w: GetAddressPoolByID: %v", ErrInvalidData, err)
		}
		pools = append(pools, pool)
	}

	if len(pools) > 1 {
		if err := addresspools.VerifyPoolsNotOverlap(pools); err != nil {
			return 0, fmt.Errorf("%w: VerifyPoolsNotOverlap: %v", ErrInvalidData, err)
		}
	}

	var dtID int

	err = db.WithTx(queryCtx, m.pg.Conn, func(q *sqlc.Queries) error {
		insertedDT, err := q.InsertDeviceTemplate(ctx, sqlc.InsertDeviceTemplateParams{
			UserID:        userID,
			InterfaceName: dt.InterfaceName,
			ListenPort:    db.IntToPtr(dt.ListenPort),
			Mtu:           db.IntToPtr(dt.MTU),
		})
		if err != nil {
			return fmt.Errorf("InsertDeviceTemplate: %w", err)
		}

		for _, p := range dt.AddressPools {
			_, err := q.InsertDeviceTemplateAddressPool(ctx, sqlc.InsertDeviceTemplateAddressPoolParams{
				DeviceTemplateID: insertedDT.ID,
				AddressPoolID:    p.ID,
			})
			if err != nil {
				return fmt.Errorf("InsertDeviceTemplateAddressPool: %w", err)
			}
		}

		for _, dns := range dt.DNS {
			_, err := q.InsertDeviceTemplateWireGuardIfaceDNS(ctx, sqlc.InsertDeviceTemplateWireGuardIfaceDNSParams{
				DeviceTemplateID: insertedDT.ID,
				Dns:              dns,
			})
			if err != nil {
				return fmt.Errorf("InsertDeviceTemplateWireGuardIfaceDNS: %w", err)
			}
		}

		dtID = insertedDT.ID
		return nil
	})
	if err != nil {
		return 0, m.handleError(fmt.Errorf("%w: %v", pg.ErrQueryFailed, err))
	}

	m.logger.Debug("A new device template saved")
	return dtID, nil
}

func (m *postgresBasedVCM) UpdateDeviceTemplateByUserID(ctx context.Context, userID int, apiDT *api.DeviceTemplate) error {
	dtNew, err := pg.NewDeviceTemplateFromAPI(apiDT)
	if err != nil {
		return fmt.Errorf("%w : %v", ErrInvalidData, err)
	}
	dtOld, err := getDeviceTemplateByUserID(ctx, m.sqlcQ, userID)
	if err != nil {
		return m.handleError(err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	pools := make([]sqlc.AddressPool, 0, len(dtNew.AddressPools))
	for _, ap := range dtNew.AddressPools {
		pool, err := m.sqlcQ.GetAddressPoolByID(queryCtx, ap.ID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("%w: address pool with ID %d does not exist", ErrInvalidData, ap.ID)
			}
			return fmt.Errorf("%w: GetAddressPoolByID: %v", ErrInvalidData, err)
		}
		pools = append(pools, pool)
	}

	if len(pools) > 1 {
		if err := addresspools.VerifyPoolsNotOverlap(pools); err != nil {
			return fmt.Errorf("%w: VerifyPoolsNotOverlap: %v", ErrInvalidData, err)
		}
	}

	var templateID int
	queryCtx, cancel = context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()
	err = db.WithTx(queryCtx, m.pg.Conn, func(qsqlc *sqlc.Queries) error {
		updatedCfg, err := qsqlc.UpdateDeviceTemplateByUserID(queryCtx, sqlc.UpdateDeviceTemplateByUserIDParams{
			UserID:        userID,
			InterfaceName: dtNew.InterfaceName,
			ListenPort:    db.IntToPtr(dtNew.ListenPort),
			Mtu:           db.IntToPtr(dtNew.MTU),
		})
		if err != nil {
			return fmt.Errorf("UpdateDeviceTemplateByUserID: %w", err)
		}

		templateID = updatedCfg.ID

		err = qsqlc.DeleteDeviceTemplateAddressPools(queryCtx, templateID)
		if err != nil {
			return fmt.Errorf("DeleteDeviceTemplateAddressPools: %w", err)
		}

		for _, p := range dtNew.AddressPools {
			_, err := qsqlc.InsertDeviceTemplateAddressPool(queryCtx, sqlc.InsertDeviceTemplateAddressPoolParams{
				DeviceTemplateID: templateID,
				AddressPoolID:    p.ID,
			})
			if err != nil {
				return fmt.Errorf("InsertDeviceTemplateAddressPool: %w", err)
			}
		}

		err = qsqlc.DeleteDeviceTemplateWireGuardDNS(queryCtx, templateID)
		if err != nil {
			return fmt.Errorf("DeleteDeviceTemplateWireGuardDNS: %w", err)
		}

		for _, dns := range dtNew.DNS {
			_, err := qsqlc.InsertDeviceTemplateWireGuardIfaceDNS(queryCtx, sqlc.InsertDeviceTemplateWireGuardIfaceDNSParams{
				DeviceTemplateID: templateID,
				Dns:              dns,
			})
			if err != nil {
				return fmt.Errorf("InsertDeviceTemplateWireGuardIfaceDNS: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return m.handleError(fmt.Errorf("%w: %v", pg.ErrQueryFailed, err))
	}

	err = m.DeleteInvalidSessionsByDeviceTemplate(ctx, templateID, dtOld.ToAPI(), dtNew.ToAPI())
	if err != nil {
		return m.handleError(err)
	}

	m.logger.Debug("Device template updated")
	return nil
}

func (m *postgresBasedVCM) DeleteInvalidSessionsByDeviceTemplate(ctx context.Context, templateID int, old, new *api.DeviceTemplate) error {
	if new.InterfaceName != old.InterfaceName ||
		new.ListenPort != old.ListenPort ||
		new.MTU != old.MTU ||
		!reflect.DeepEqual(new.DNS, old.DNS) {

		if err := deleteTemplateDevicesSessions(ctx, m.sqlcQ, templateID); err != nil {
			return err
		}
	}

	return nil
}

func (m *postgresBasedVCM) DeleteInvalidSessionsByAdjacency(ctx context.Context, old, new *sqlc.Adjacency) error {
	// if adjacency has been reassigned to a different device, invalidate sessions for both devices ...
	if new.DeviceID != old.DeviceID {
		if _, err := m.sqlcQ.InvalidateDeviceSession(ctx, old.DeviceID); err != nil {
			return err
		}
		if _, err := m.sqlcQ.InvalidateDeviceSession(ctx, new.DeviceID); err != nil {
			return err
		}
		return nil
	}

	// ... otherwise compare attributes as usual
	oldPresharedKey, err := aesgcm.OpenString(m.encryptionKey, old.PresharedKeyEncrypted)
	if err != nil {
		return err
	}
	newPresharedKey, err := aesgcm.OpenString(m.encryptionKey, new.PresharedKeyEncrypted)
	if err != nil {
		return err
	}

	if new.ServerID != old.ServerID ||
		!reflect.DeepEqual(new.ClientSideAllowedIps, old.ClientSideAllowedIps) ||
		newPresharedKey != oldPresharedKey {

		if _, err := m.sqlcQ.InvalidateDeviceSession(ctx, new.DeviceID); err != nil {
			return err
		}
	}
	return nil
}

func (m *postgresBasedVCM) DeleteInvalidSessionsByServer(ctx context.Context, old, new *pg.Server) error {
	hcUnchanged := (old.HealthCheckAddress == nil && new.HealthCheckAddress == nil) ||
		(old.HealthCheckAddress != nil && new.HealthCheckAddress != nil && *new.HealthCheckAddress == *old.HealthCheckAddress)

	if new.Endpoint != old.Endpoint ||
		!hcUnchanged {

		if _, err := m.sqlcQ.InvalidateAllDeviceSessionsByServerID(ctx, new.ID); err != nil {
			return err
		}
	}

	return nil
}

func (m *postgresBasedVCM) DeleteInvalidSessionsByServerWGConfig(ctx context.Context, serverID int, old, new *pg.WireGuardConfig) error {
	if new.PrivateKey != old.PrivateKey ||
		new.PublicKey != old.PublicKey ||
		new.ListenPort != old.ListenPort ||
		new.PersistentKeepalive != old.PersistentKeepalive {

		if _, err := m.sqlcQ.InvalidateAllDeviceSessionsByServerID(ctx, serverID); err != nil {
			return err
		}
	}

	return nil
}

func (m *postgresBasedVCM) UpdateWireGuardServerConfig(ctx context.Context, serverID int, c *api.ServerWireGuardInterface) error {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	oldWGC, err := GetWireGuardServerConfig(queryCtx, m.sqlcQ, m.encryptionKey, serverID)
	if err != nil {
		return m.handleError(err)
	}
	wgc, err := pg.NewWireGuardConfigFromAPI(c)
	if err != nil {
		return fmt.Errorf("%w : %v", ErrInvalidData, err)
	}

	adjacenciesCovered := true
	addressColision := false
	serverAdjacencies, err := GetWireGuardServerAdjacencies(queryCtx, m.pg.Conn, m.encryptionKey, serverID)
	if err != nil {
		return m.handleError(err)
	}
	// consistency checks:
	// - every existing adjacency must be covered by some of the newly configured addresses (of server interface)
	// - device and server addresses can not be the same
	for _, p := range serverAdjacencies {
		for _, deviceAddress := range p.AllowedIPs {
			deviceNet, err := netip.ParsePrefix(deviceAddress)
			if err != nil {
				m.logger.
					WithError(err).
					WithField("deviceAddress", deviceAddress).
					Warn("Could not parse device's address")
				continue
			}
			deviceAddr := deviceNet.Addr()

			adjacencyCovered := false
			for _, serverAddress := range wgc.Addresses {
				serverNet, err := netip.ParsePrefix(serverAddress)
				if err != nil {
					m.logger.
						WithError(err).
						WithField("serverAddress", serverAddress).
						Warn("Could not parse server's address")
					continue
				}

				if serverNet.Contains(deviceAddr) {
					adjacencyCovered = true
				}
				if serverNet.Addr() == deviceAddr {
					addressColision = true
					break
				}
			}
			if !adjacencyCovered {
				adjacenciesCovered = false
				break
			}
		}
	}

	if !adjacenciesCovered {
		return errors.New("existing adjacencies not covered by proposed server interface addresses")
	}
	if addressColision {
		return errors.New("device and server addresses clash")
	}

	privEnc, err := aesgcm.Seal(m.encryptionKey, wgc.PrivateKey)
	if err != nil {
		return errors.New("failed to encrypt private key")
	}

	err = db.WithTx(queryCtx, m.pg.Conn, func(qsqlc *sqlc.Queries) error {
		updatedCfg, err := qsqlc.UpdateServerWireGuardConfig(queryCtx, sqlc.UpdateServerWireGuardConfigParams{
			ServerID:            serverID,
			InterfaceName:       wgc.InterfaceName,
			PrivateKeyEncrypted: privEnc,
			PublicKey:           wgc.PublicKey,
			ListenPort:          db.IntToPtr(wgc.ListenPort),
			Mtu:                 db.IntToPtr(wgc.MTU),
			PersistentKeepalive: db.IntToPtr(wgc.PersistentKeepalive),
		})
		if err != nil {
			return fmt.Errorf("UpdateServerWireGuardConfig: %w", err)
		}

		err = qsqlc.DeleteServerWireGuardIfaceAddrs(queryCtx, updatedCfg.ID)
		if err != nil {
			return fmt.Errorf("DeleteServerWireGuardIfaceAddrs: %w", err)
		}

		for _, addr := range wgc.Addresses {
			parsedPrefix, err := netip.ParsePrefix(addr)
			if err != nil {
				return fmt.Errorf("parse address '%s': %w", addr, err)
			}
			_, err = qsqlc.InsertServerWireGuardIfaceAddr(queryCtx, sqlc.InsertServerWireGuardIfaceAddrParams{
				ServerConfigID: updatedCfg.ID,
				Addr:           parsedPrefix,
			})
			if err != nil {
				return fmt.Errorf("InsertServerWireGuardIfaceAddr: %w", err)
			}
		}

		err = qsqlc.DeleteServerWireGuardDNS(queryCtx, updatedCfg.ID)
		if err != nil {
			return fmt.Errorf("DeleteServerWireGuardDNS: %w", err)
		}

		for _, dns := range wgc.DNS {
			_, err := qsqlc.InsertServerWireGuardIfaceDNS(queryCtx, sqlc.InsertServerWireGuardIfaceDNSParams{
				ServerConfigID: updatedCfg.ID,
				Dns:            dns,
			})
			if err != nil {
				return fmt.Errorf("InsertServerWireGuardIfaceDNS: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return m.handleError(fmt.Errorf("%w: %v", pg.ErrQueryFailed, err))
	}
	m.logger.Debug("A WireGuard server configuration updated")

	err = m.DeleteInvalidSessionsByServerWGConfig(queryCtx, serverID, oldWGC, wgc)
	if err != nil {
		return m.handleError(fmt.Errorf("%w: %v", pg.ErrQueryFailed, err))
	}
	return nil
}

func (m *postgresBasedVCM) GetWireGuardServerAdjacencies(ctx context.Context, serverID int) ([]*api.WireGuardPeer, error) {
	adjacencies, err := GetWireGuardServerAdjacencies(ctx, m.pg.Conn, m.encryptionKey, serverID)
	if err != nil {
		return nil, m.handleError(err)
	}
	list := make([]*api.WireGuardPeer, 0, len(adjacencies))
	for _, p := range adjacencies {
		list = append(list, p.ToAPI())
	}
	return list, nil
}

func (m *postgresBasedVCM) GetWireGuardDeviceAdjacencies(ctx context.Context, deviceID int) ([]*api.WireGuardPeer, error) {
	adjacencies, err := GetWireGuardDeviceAdjacencies(ctx, m.pg.Conn, m.encryptionKey, deviceID)
	if err != nil {
		return nil, m.handleError(err)
	}

	list := make([]*api.WireGuardPeer, 0, len(adjacencies))
	for _, p := range adjacencies {
		apiPeer := p.ToAPI()
		list = append(list, apiPeer)
	}

	return list, nil
}

func (m *postgresBasedVCM) GetUserDeviceAdjacencies(ctx context.Context, userID int) ([]*api.AdjacencyExtended, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	userExists, err := m.sqlcQ.UserExists(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: UserExists: %v", pg.ErrQueryFailed, err)
	}
	if !userExists {
		return nil, fmt.Errorf("%w: user with ID %d", ErrDoesNotExist, userID)
	}

	devices, err := m.sqlcQ.GetDevicesByUserID(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetDevicesByUserID: %v", pg.ErrQueryFailed, err)
	}

	var adjacencies []*api.AdjacencyExtended
	for _, device := range devices {
		deviceAdjacencies, err := GetWireGuardDeviceAdjacencies(queryCtx, m.pg.Conn, m.encryptionKey, int(device.ID))
		if err != nil {
			return nil, fmt.Errorf("%w: GetWireGuardDeviceAdjacencies: %v", pg.ErrQueryFailed, err)
		}

		deviceAddresses, err := m.sqlcQ.GetDeviceWireGuardIfaceAddrs(queryCtx, int(device.ID))
		if err != nil {
			return nil, fmt.Errorf("%w: GetDeviceWireGuardIfaceAddrs: %v", pg.ErrQueryFailed, err)
		}

		for _, peer := range deviceAdjacencies {
			apiPeer := peer.ToAPI()
			apiDevice, err := pg.ConvertDBToAPIDevice(m.encryptionKey, device, deviceAddresses)
			if err != nil {
				return nil, err
			}

			adjacencies = append(adjacencies, &api.AdjacencyExtended{
				ID:     peer.ID,
				Server: apiPeer.Server,
				Device: apiDevice,
				Config: apiPeer.Config,
			})
		}
	}

	return adjacencies, nil
}

func (m *postgresBasedVCM) CreateAdjacency(ctx context.Context, serverID, deviceID int, wgAdj *api.WireGuardAdjacency) error {
	config, err := pg.NewWireGuardAdjacencyConfigFromAPI(wgAdj)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidData, err)
	}

	server, err := m.pg.GetServerByID(ctx, serverID)
	if err != nil {
		return m.handleError(err)
	}

	// automatically ensure healthcheck address is in allowed IPs
	config.ClientSideAllowedIPs, _ = EnsureHealthcheckAddrInAllowedIPs(config.ClientSideAllowedIPs, server.HealthCheckAddress)

	exists, err := m.sqlcQ.AdjacencyExists(ctx, sqlc.AdjacencyExistsParams{
		ServerID: serverID,
		DeviceID: deviceID,
	})
	if err != nil {
		return m.handleError(err)
	}
	if exists {
		return ErrAlreadyExists
	}

	wgcDevice, err := getDeviceWireGuardConfig(ctx, m.sqlcQ, m.encryptionKey, deviceID)
	if err != nil {
		return m.handleError(err)
	}
	if len(wgcDevice.Addresses) == 0 {
		return fmt.Errorf("%w: device's interface addresses list is empty", ErrInvalidData)
	}

	wgcServer, err := GetWireGuardServerConfig(ctx, m.sqlcQ, m.encryptionKey, serverID)
	if err != nil {
		return m.handleError(err)
	}
	if len(wgcServer.Addresses) == 0 {
		return fmt.Errorf("%w: server's interface addresses list is empty", ErrInvalidData)
	}

	deviceAdjacencies, err := m.sqlcQ.GetDeviceWireGuardAdjacencies(ctx, deviceID)
	if err != nil {
		return m.handleError(err)
	}
	for _, p := range deviceAdjacencies {
		for _, peerAllowedIP := range p.ClientSideAllowedIps {
			for _, newAllowedIP := range config.ClientSideAllowedIPs {
				if peerAllowedIP == newAllowedIP {
					return fmt.Errorf("%w: device already has adjacency with the same allowed IP %q", ErrInvalidData, newAllowedIP)
				}
			}
		}
	}

	serverSideAllowedIP, err := m.getServerSideAllowedIP(wgcServer.Addresses, wgcDevice.Addresses)
	if err != nil {
		return err
	}

	serverAdjacencies, err := GetWireGuardServerAdjacencies(ctx, m.pg.Conn, m.encryptionKey, serverID)
	if err != nil {
		return m.handleError(err)
	}
	for _, p := range serverAdjacencies {
		for _, peerAllowedIP := range p.AllowedIPs {
			if peerAllowedIP == serverSideAllowedIP.String() {
				return fmt.Errorf("%w: server already has adjacency with the same IP address as device's interface address (%q)", ErrInvalidData, serverSideAllowedIP)
			}
		}
	}

	found := false
	for _, ip := range config.ServerSideAllowedIPs {
		if serverSideAllowedIP == ip {
			found = true
			break
		}
	}

	if !found {
		config.ServerSideAllowedIPs = append(config.ServerSideAllowedIPs, serverSideAllowedIP)
	}

	pskEnc, err := aesgcm.Seal(m.encryptionKey, config.PresharedKey)
	if err != nil {
		return m.handleError(err)
	}

	if _, err := m.sqlcQ.InsertAdjacency(ctx, sqlc.InsertAdjacencyParams{
		ServerID:              serverID,
		DeviceID:              deviceID,
		ServerSideAllowedIps:  config.ServerSideAllowedIPs,
		ClientSideAllowedIps:  config.ClientSideAllowedIPs,
		PresharedKeyEncrypted: pskEnc,
	}); err != nil {
		return m.handleError(err)
	}
	m.logger.WithFields(log.Fields{
		"serverID": serverID,
		"deviceID": deviceID,
	}).Debug("A new WireGuard peer config added")

	_, err = m.sqlcQ.InvalidateDeviceSession(ctx, deviceID)
	if err != nil {
		return m.handleError(err)
	}

	return nil
}

func (m *postgresBasedVCM) UpdateAdjacency(ctx context.Context, adjacencyID int, serverID, deviceID int, wgAdj *api.WireGuardAdjacency) error {
	adjacency, err := m.sqlcQ.GetAdjacency(ctx, adjacencyID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrDoesNotExist
		}
		return m.handleError(err)
	}

	oldServerID := adjacency.ServerID
	oldDeviceID := adjacency.DeviceID

	server, err := m.pg.GetServerByID(ctx, serverID)
	if err != nil {
		return m.handleError(err)
	}
	config, err := pg.NewWireGuardAdjacencyConfigFromAPI(wgAdj)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidData, err)
	}

	// automatically ensure healthcheck address is in allowed IPs
	config.ClientSideAllowedIPs, _ = EnsureHealthcheckAddrInAllowedIPs(config.ClientSideAllowedIPs, server.HealthCheckAddress)

	if deviceID != oldDeviceID || serverID != oldServerID {
		exists, err := m.sqlcQ.AdjacencyExists(ctx, sqlc.AdjacencyExistsParams{
			ServerID: serverID,
			DeviceID: deviceID,
		})
		if err != nil {
			return m.handleError(err)
		}
		if exists {
			return ErrAlreadyExists
		}
	}

	oldWgcDevice, err := getDeviceWireGuardConfig(ctx, m.sqlcQ, m.encryptionKey, oldDeviceID)
	if err != nil {
		return m.handleError(err)
	}

	newWgcDevice, err := getDeviceWireGuardConfig(ctx, m.sqlcQ, m.encryptionKey, deviceID)
	if err != nil {
		return m.handleError(err)
	}
	if len(newWgcDevice.Addresses) == 0 {
		return fmt.Errorf("%w: device's interface addresses list is empty", ErrInvalidData)
	}

	oldWgcServer, err := GetWireGuardServerConfig(ctx, m.sqlcQ, m.encryptionKey, oldServerID)
	if err != nil {
		return m.handleError(err)
	}

	newWgcServer, err := GetWireGuardServerConfig(ctx, m.sqlcQ, m.encryptionKey, serverID)
	if err != nil {
		return m.handleError(err)
	}
	if len(newWgcServer.Addresses) == 0 {
		return fmt.Errorf("%w: server's interface addresses list is empty", ErrInvalidData)
	}

	deviceAdjacencies, err := m.sqlcQ.GetDeviceWireGuardAdjacencies(ctx, deviceID)
	if err != nil {
		return m.handleError(err)
	}
	for _, p := range deviceAdjacencies {
		if deviceID == oldDeviceID && p.PublicKey == oldWgcServer.PublicKey {
			continue
		}
		for _, peerAllowedIP := range p.ClientSideAllowedIps {
			for _, newAllowedIP := range config.ClientSideAllowedIPs {
				if peerAllowedIP == newAllowedIP {
					return fmt.Errorf("%w: device already has adjacency with the same allowed IP %q", ErrInvalidData, newAllowedIP)
				}
			}
		}
	}

	serverSideAllowedIP, err := m.getServerSideAllowedIP(newWgcServer.Addresses, newWgcDevice.Addresses)
	if err != nil {
		return err
	}

	serverAdjacencies, err := GetWireGuardServerAdjacencies(ctx, m.pg.Conn, m.encryptionKey, serverID)
	if err != nil {
		return m.handleError(err)
	}
	for _, p := range serverAdjacencies {
		if serverID == oldServerID && p.PublicKey == oldWgcDevice.PublicKey {
			continue
		}
		for _, peerAllowedIP := range p.AllowedIPs {
			if peerAllowedIP == serverSideAllowedIP.String() {
				return fmt.Errorf("%w: server already has adjacency with the same IP address as device's interface address (%q)", ErrInvalidData, serverSideAllowedIP)
			}
		}
	}

	pskEnc, err := aesgcm.Seal(m.encryptionKey, config.PresharedKey)
	if err != nil {
		return m.handleError(err)
	}

	updatedAdj, err := m.sqlcQ.UpdateAdjacency(ctx, sqlc.UpdateAdjacencyParams{
		ID:                    adjacencyID,
		ServerID:              serverID,
		DeviceID:              deviceID,
		PresharedKeyEncrypted: pskEnc,
		ServerSideAllowedIps:  config.ServerSideAllowedIPs,
		ClientSideAllowedIps:  config.ClientSideAllowedIPs,
	})
	if err != nil {
		return m.handleError(err)
	}
	m.logger.WithFields(log.Fields{
		"adjacencyID": adjacencyID,
		"deviceID":    deviceID,
		"serverID":    serverID,
	}).Debug("WireGuard peer config updated")

	err = m.DeleteInvalidSessionsByAdjacency(ctx, &adjacency, &updatedAdj)
	if err != nil {
		return m.handleError(err)
	}

	return nil
}

func (m *postgresBasedVCM) DeleteAdjacency(ctx context.Context, adjacencyID int) error {
	adjacency, err := m.sqlcQ.GetAdjacency(ctx, adjacencyID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return m.handleError(err)
	}

	if err := m.sqlcQ.DeleteAdjacency(ctx, adjacencyID); err != nil {
		return m.handleError(err)
	}

	_, err = m.sqlcQ.InvalidateDeviceSession(ctx, adjacency.DeviceID)
	if err != nil {
		return m.handleError(err)
	}

	return nil
}

func (m *postgresBasedVCM) GetUserAdjacencyTemplates(ctx context.Context, userID int) ([]*api.AdjacencyTemplateExtended, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	userExists, err := m.sqlcQ.UserExists(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: UserExists: %v", pg.ErrQueryFailed, err)
	}
	if !userExists {
		return nil, fmt.Errorf("%w: user with ID %d", ErrDoesNotExist, userID)
	}

	adjacencyTemplates, err := m.sqlcQ.GetUserAdjacencyTemplates(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetUserAdjacencyTemplates: %v", pg.ErrQueryFailed, err)
	}

	var adjTemplates []*api.AdjacencyTemplateExtended
	for _, at := range adjacencyTemplates {
		server := &api.Server{
			ID:          at.ServerID,
			Name:        at.ServerName,
			Endpoint:    at.ServerEndpoint.String(),
			Description: at.ServerDescription,
		}
		if at.ServerHealthcheckAddress != nil {
			server.HealthCheckAddress = at.ServerHealthcheckAddress.String()
		}

		config := &api.WireGuardAdjacencyTemplate{
			UsePresharedKey: at.UsePresharedKey,
		}
		for _, ip := range at.ClientSideAllowedIps {
			config.ClientSideAllowedIPs = append(config.ClientSideAllowedIPs, ip.String())
		}

		adjTemplates = append(adjTemplates, &api.AdjacencyTemplateExtended{
			ID:             at.ID,
			Server:         server,
			TemplateConfig: config,
		})
	}

	return adjTemplates, nil
}

func (m *postgresBasedVCM) CreateAdjacencyTemplate(ctx context.Context, server *api.Server, userID int, wgAdj *api.WireGuardAdjacencyTemplate) error {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	exists, err := m.sqlcQ.AdjacencyTemplateExists(queryCtx, sqlc.AdjacencyTemplateExistsParams{
		ServerID: server.ID,
		UserID:   userID,
	})
	if err != nil {
		return m.handleError(fmt.Errorf("%w: AdjacencyTemplateExists: %v", pg.ErrQueryFailed, err))
	}
	if exists {
		return ErrAlreadyExists
	}

	config, err := pg.NewWireGuardAdjacencyTemplateConfigFromAPI(wgAdj)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidData, err)
	}

	// automatically ensure healthcheck address is in allowed IPs if present
	if server.HealthCheckAddress != "" {
		addr, err := netip.ParseAddr(server.HealthCheckAddress)
		if err != nil {
			return fmt.Errorf("parse healthcheck address '%s': %w", server.HealthCheckAddress, err)
		}
		config.ClientSideAllowedIPs, _ = EnsureHealthcheckAddrInAllowedIPs(config.ClientSideAllowedIPs, &addr)
	}

	// check for duplicate client-side allowed IPs with other adjacency templates for this user
	userAdjTemplates, err := m.sqlcQ.GetUserAdjacencyTemplates(queryCtx, userID)
	if err != nil {
		return m.handleError(fmt.Errorf("%w: GetUserAdjacencyTemplates: %v", pg.ErrQueryFailed, err))
	}
	for _, adjTempl := range userAdjTemplates {
		for _, existingAllowedIP := range adjTempl.ClientSideAllowedIps {
			for _, newAllowedIP := range config.ClientSideAllowedIPs {
				if existingAllowedIP == newAllowedIP {
					return fmt.Errorf("%w: user already has adjacency template with the same allowed IP %q", ErrInvalidData, newAllowedIP.String())
				}
			}
		}
	}

	if _, err := m.sqlcQ.InsertAdjacencyTemplate(queryCtx, sqlc.InsertAdjacencyTemplateParams{
		ServerID:             server.ID,
		UserID:               userID,
		ClientSideAllowedIps: config.ClientSideAllowedIPs,
		UsePresharedKey:      config.UsePresharedKey,
	}); err != nil {
		return m.handleError(fmt.Errorf("%w: InsertAdjacencyTemplate: %v", pg.ErrQueryFailed, err))
	}

	return nil
}

func (m *postgresBasedVCM) UpdateAdjacencyTemplate(ctx context.Context, adjacencyTemplateID int, server *api.Server, userID int, wgAdj *api.WireGuardAdjacencyTemplate) error {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	currentTemplate, err := m.sqlcQ.GetAdjacencyTemplateByID(queryCtx, adjacencyTemplateID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrDoesNotExist
		}
		return m.handleError(fmt.Errorf("%w: GetAdjacencyTemplateByID: %v", pg.ErrQueryFailed, err))
	}

	if currentTemplate.UserID != userID {
		return fmt.Errorf("%w: user cannot be changed for adjacency template", ErrInvalidData)
	}

	exists, err := m.sqlcQ.AdjacencyTemplateExists(queryCtx, sqlc.AdjacencyTemplateExistsParams{
		ServerID: server.ID,
		UserID:   userID,
	})
	if err != nil {
		return m.handleError(fmt.Errorf("%w: AdjacencyTemplateExists: %v", pg.ErrQueryFailed, err))
	}
	if exists && currentTemplate.ServerID != server.ID {
		return ErrAlreadyExists
	}

	config, err := pg.NewWireGuardAdjacencyTemplateConfigFromAPI(wgAdj)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidData, err)
	}

	// automatically ensure healthcheck address is in allowed IPs if present
	if server.HealthCheckAddress != "" {
		addr, err := netip.ParseAddr(server.HealthCheckAddress)
		if err != nil {
			return fmt.Errorf("parse healthcheck address '%s': %w", server.HealthCheckAddress, err)
		}
		config.ClientSideAllowedIPs, _ = EnsureHealthcheckAddrInAllowedIPs(config.ClientSideAllowedIPs, &addr)
	}

	// check for duplicate client-side allowed IPs with other adjacency templates for this user
	userAdjTemplates, err := m.sqlcQ.GetUserAdjacencyTemplates(queryCtx, userID)
	if err != nil {
		return m.handleError(fmt.Errorf("%w: GetUserAdjacencyTemplates: %v", pg.ErrQueryFailed, err))
	}
	for _, adjTempl := range userAdjTemplates {
		if adjTempl.ID == adjacencyTemplateID {
			continue
		}
		for _, existingAllowedIP := range adjTempl.ClientSideAllowedIps {
			for _, newAllowedIP := range config.ClientSideAllowedIPs {
				if existingAllowedIP == newAllowedIP {
					return fmt.Errorf("%w: user already has adjacency template with the same allowed IP %q", ErrInvalidData, newAllowedIP.String())
				}
			}
		}
	}

	if _, err := m.sqlcQ.UpdateAdjacencyTemplate(queryCtx, sqlc.UpdateAdjacencyTemplateParams{
		ID:                   adjacencyTemplateID,
		ServerID:             server.ID,
		ClientSideAllowedIps: config.ClientSideAllowedIPs,
		UsePresharedKey:      config.UsePresharedKey,
	}); err != nil {
		return m.handleError(fmt.Errorf("%w: UpdateAdjacencyTemplate: %v", pg.ErrQueryFailed, err))
	}

	return nil
}

func (m *postgresBasedVCM) DeleteAdjacencyTemplate(ctx context.Context, adjacencyTemplateID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	exists, err := m.sqlcQ.AdjacencyTemplateExistsByID(queryCtx, adjacencyTemplateID)
	if err != nil {
		return m.handleError(fmt.Errorf("%w: AdjacencyTemplateExistsByID: %v", pg.ErrQueryFailed, err))
	}
	if !exists {
		return ErrDoesNotExist
	}

	if err := m.sqlcQ.DeleteAdjacencyTemplate(queryCtx, adjacencyTemplateID); err != nil {
		return m.handleError(fmt.Errorf("%w: DeleteAdjacencyTemplate: %v", pg.ErrQueryFailed, err))
	}

	return nil
}

func (m *postgresBasedVCM) ResyncDeviceAdjacencies(ctx context.Context, userID int) (*api.DeviceAdjacencyResyncResponse, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	userExists, err := m.sqlcQ.UserExists(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: UserExists: %v", pg.ErrQueryFailed, err)
	}
	if !userExists {
		return nil, fmt.Errorf("%w: user with ID %d", ErrDoesNotExist, userID)
	}

	response := &api.DeviceAdjacencyResyncResponse{}

	userAdjTemplates, err := m.sqlcQ.GetUserAdjacencyTemplates(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetUserAdjacencyTemplates: %v", pg.ErrQueryFailed, err)
	}

	// map of adjacency templates by server ID for quick lookup
	adjTemplatesByServerID := make(map[int]*sqlc.GetUserAdjacencyTemplatesRow)
	for _, adjTemplate := range userAdjTemplates {
		adjTemplatesByServerID[adjTemplate.ServerID] = &adjTemplate
	}

	devices, err := m.sqlcQ.GetDevicesByUserID(queryCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetDevicesByUserID: %v", pg.ErrQueryFailed, err)
	}

	// process each device adjacency (belonging to the affected user)
	for _, device := range devices {
		devicePeers, err := m.sqlcQ.GetDeviceWireGuardAdjacencies(queryCtx, device.ID)
		if err != nil {
			response.Errors = append(response.Errors, fmt.Sprintf("Getting adjacencies for device %d: %v", device.ID, err))
			continue
		}

		// track which servers already have adjacencies for this device
		existingAdjByServerID := make(map[int]*sqlc.GetDeviceWireGuardAdjacenciesRow)
		for _, peer := range devicePeers {
			existingAdjByServerID[peer.Serverid] = &peer
		}

		// for each existing device adjacency, check if it should be deleted or updated
		for serverID, existingAdj := range existingAdjByServerID {
			adjTemplate, hasAdjTemplate := adjTemplatesByServerID[serverID]

			if !hasAdjTemplate {
				// no adjacency template with the same server - delete the device adjacency
				if err := m.sqlcQ.DeleteAdjacency(ctx, existingAdj.ID); err != nil {
					response.Errors = append(response.Errors, fmt.Sprintf("Deleting adjacency for device %d, server %d: %v", device.ID, serverID, err))
					continue
				}
				response.AdjacenciesDeleted++

				m.logger.WithFields(log.Fields{
					"deviceID": device.ID,
					"serverID": serverID,
				}).Debug("Deleted device adjacency during resync")

				_, err = m.sqlcQ.InvalidateDeviceSession(ctx, device.ID)
				if err != nil {
					response.Errors = append(response.Errors, fmt.Sprintf("Invalidating session for device %d: %v", device.ID, err))
					continue
				}
				continue
			}
			// adjacency template exists - recompute server side allowed IPs and compare
			wgcDevice, err := getDeviceWireGuardConfig(ctx, m.sqlcQ, m.encryptionKey, device.ID)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Getting device config for device %d: %v", device.ID, err))
				continue
			}

			server, err := m.pg.GetServerByID(ctx, serverID)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Getting server %d: %v", serverID, err))
				continue
			}

			wgcServer, err := GetWireGuardServerConfig(ctx, m.sqlcQ, m.encryptionKey, serverID)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Getting server config for server %d: %v", serverID, err))
				continue
			}

			// recompute server side allowed IPs
			serverSideAllowedIP, err := m.getServerSideAllowedIP(wgcServer.Addresses, wgcDevice.Addresses)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Computing server side allowed IP for device %d, server %d: %v", device.ID, serverID, err))
				continue
			}

			desiredServerSideAllowedIPs := []netip.Prefix{serverSideAllowedIP}

			// is healthcheck address in client side allowed IPs
			contains := IsHealthcheckAddrInAllowedIPs(adjTemplate.ClientSideAllowedIps, server.HealthCheckAddress)
			if !contains {
				response.Errors = append(response.Errors, fmt.Sprintf("Healthcheck address %s not in client side allowed IPs for device %d, server %d", server.HealthCheckAddress.String(), device.ID, serverID))
				continue
			}

			needsUpdate := false
			if len(existingAdj.ServerSideAllowedIps) != len(desiredServerSideAllowedIPs) {
				needsUpdate = true
			} else {
				for i, ip := range existingAdj.ServerSideAllowedIps {
					if ip != desiredServerSideAllowedIPs[i] {
						needsUpdate = true
						break
					}
				}
			}

			if len(existingAdj.ClientSideAllowedIps) != len(adjTemplate.ClientSideAllowedIps) {
				needsUpdate = true
			} else {
				for i, ip := range existingAdj.ClientSideAllowedIps {
					if ip != adjTemplate.ClientSideAllowedIps[i] {
						needsUpdate = true
						break
					}
				}
			}

			existingPresharedKey, err := aesgcm.OpenString(m.encryptionKey, existingAdj.PresharedKeyEncrypted)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("failed to decrypt existing preshared key for device %d, server %d: %v", device.ID, serverID, err))
				continue
			}

			var desiredPresharedKey string
			if !adjTemplate.UsePresharedKey {
				desiredPresharedKey = ""
				needsUpdate = needsUpdate || (existingPresharedKey != "")
			} else if existingPresharedKey != "" {
				desiredPresharedKey = existingPresharedKey
			} else {
				needsUpdate = true
				newPsk, err := keys.GenerateBase64EncodedWGPresharedKey()
				if err != nil {
					response.Errors = append(response.Errors, fmt.Sprintf("failed to generate preshared key for device %d, server %d: %v", device.ID, serverID, err))
					continue
				}
				desiredPresharedKey = newPsk
			}

			pskEnc, err := aesgcm.Seal(m.encryptionKey, desiredPresharedKey)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("failed to encrypt preshared key for device %d, server %d: %v", device.ID, serverID, err))
				continue
			}

			if needsUpdate {
				updatedAdjacency, err := m.sqlcQ.UpdateAdjacency(ctx, sqlc.UpdateAdjacencyParams{
					ID:                    existingAdj.ID,
					ServerID:              serverID,
					DeviceID:              device.ID,
					ServerSideAllowedIps:  desiredServerSideAllowedIPs,
					ClientSideAllowedIps:  adjTemplate.ClientSideAllowedIps,
					PresharedKeyEncrypted: pskEnc,
				})
				if err != nil {
					response.Errors = append(response.Errors, fmt.Sprintf("Updating adjacency for device %d, server %d: %v", device.ID, serverID, err))
					continue
				}
				response.AdjacenciesUpdated++

				m.logger.WithFields(log.Fields{
					"deviceID": device.ID,
					"serverID": serverID,
				}).Debug("Updated device adjacency during resync")

				err = m.DeleteInvalidSessionsByAdjacency(ctx, &sqlc.Adjacency{
					ServerID:              serverID,
					DeviceID:              device.ID,
					ServerSideAllowedIps:  existingAdj.ServerSideAllowedIps,
					ClientSideAllowedIps:  existingAdj.ClientSideAllowedIps,
					PresharedKeyEncrypted: existingAdj.PresharedKeyEncrypted,
				}, &updatedAdjacency)
				if err != nil {
					response.Errors = append(response.Errors, fmt.Sprintf("Deleting invalid sessions: %v", err))
					continue
				}
			}
		}

		// for each adjacency template, check if there's a device adjacency; if not, create it
		for serverID, adjTemplate := range adjTemplatesByServerID {
			if _, exists := existingAdjByServerID[serverID]; exists {
				// adjacency already exists (we handled it above)
				continue
			}

			// create a new device adjacency
			wgcDevice, err := getDeviceWireGuardConfig(ctx, m.sqlcQ, m.encryptionKey, device.ID)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Getting device config for device %d: %v", device.ID, err))
				continue
			}

			server, err := m.pg.GetServerByID(ctx, serverID)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Getting server %d: %v", serverID, err))
				continue
			}

			wgcServer, err := GetWireGuardServerConfig(ctx, m.sqlcQ, m.encryptionKey, serverID)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Getting server config for server %d: %v", serverID, err))
				continue
			}

			// compute server side allowed IP
			serverSideAllowedIP, err := m.getServerSideAllowedIP(wgcServer.Addresses, wgcDevice.Addresses)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Computing server side allowed IP for device %d, server %d: %v", device.ID, serverID, err))
				continue
			}

			// is healthcheck address in client side allowed IPs
			contains := IsHealthcheckAddrInAllowedIPs(adjTemplate.ClientSideAllowedIps, server.HealthCheckAddress)
			if !contains {
				response.Errors = append(response.Errors, fmt.Sprintf("Healthcheck address %s not in client side allowed IPs for device %d, server %d", server.HealthCheckAddress.String(), device.ID, serverID))
				continue
			}

			desiredPresharedKey := ""
			if adjTemplate.UsePresharedKey {
				newPsk, err := keys.GenerateBase64EncodedWGPresharedKey()
				if err != nil {
					response.Errors = append(response.Errors, fmt.Sprintf("failed to generate preshared key for device %d, server %d: %v", device.ID, serverID, err))
					continue
				}
				desiredPresharedKey = newPsk
			}

			pskEnc, err := aesgcm.Seal(m.encryptionKey, desiredPresharedKey)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("failed to encrypt preshared key for device %d, server %d: %v", device.ID, serverID, err))
				continue
			}

			if _, err := m.sqlcQ.InsertAdjacency(ctx, sqlc.InsertAdjacencyParams{
				ServerID:              serverID,
				DeviceID:              device.ID,
				ServerSideAllowedIps:  []netip.Prefix{serverSideAllowedIP},
				ClientSideAllowedIps:  adjTemplate.ClientSideAllowedIps,
				PresharedKeyEncrypted: pskEnc,
			}); err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Creating adjacency for device %d, server %d: %v", device.ID, serverID, err))
				continue
			}
			response.AdjacenciesCreated++

			m.logger.WithFields(log.Fields{
				"deviceID": device.ID,
				"serverID": serverID,
			}).Debug("Created device adjacency during resync")

			_, err = m.sqlcQ.InvalidateDeviceSession(ctx, device.ID)
			if err != nil {
				response.Errors = append(response.Errors, fmt.Sprintf("Invalidating session for device %d: %v", device.ID, err))
				continue
			}
		}
	}

	return response, nil
}

func (m *postgresBasedVCM) DeleteDeviceTemplateByUserID(ctx context.Context, userID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	deviceTemplate, err := m.sqlcQ.GetDeviceTemplateByUserID(queryCtx, userID)
	if err != nil && err != pgx.ErrNoRows {
		return m.handleError(fmt.Errorf("%w: GetDeviceTemplateByUserID: %v", pg.ErrQueryFailed, err))
	}

	devices, err := m.sqlcQ.GetDevicesByTemplateID(queryCtx, deviceTemplate.ID)
	if err != nil && err != pgx.ErrNoRows {
		return m.handleError(fmt.Errorf("%w: GetDevicesByTemplateID: %v", pg.ErrQueryFailed, err))
	}

	err = db.WithTx(queryCtx, m.pg.Conn, func(q *sqlc.Queries) error {
		// delete device adjacencies to satisfy DB constraint
		for _, d := range devices {
			peers, err := q.GetDeviceWireGuardAdjacencies(ctx, d.ID)
			if err != nil {
				return m.handleError(err)
			}
			for _, p := range peers {
				if err := q.DeleteAdjacency(ctx, p.ID); err != nil {
					m.logger.
						WithField("deviceID", d.ID).
						WithField("serverID", p.Serverid).
						Info("Failed to remove WireGuard adjacency between device and server")
				}
			}
		}

		// clear sessions for all devices under this template to trigger healtcheck sessions updates
		if err := deleteTemplateDevicesSessions(ctx, q, deviceTemplate.ID); err != nil {
			return m.handleError(fmt.Errorf("%w: deleteTemplateDevicesSessions: %v", pg.ErrQueryFailed, err))
		}

		// this also deletes device(s) and all related data because of foreign keys with ON DELETE CASCADE
		if err := q.DeleteDeviceTemplate(queryCtx, deviceTemplate.ID); err != nil {
			return m.handleError(fmt.Errorf("%w: DeleteDeviceTemplate: %v", pg.ErrQueryFailed, err))
		}
		return nil
	})
	if err != nil {
		return m.handleError(fmt.Errorf("%w: %v", pg.ErrQueryFailed, err))
	}
	return nil
}

func (m *postgresBasedVCM) WatchWireGuardConfig(ctx context.Context, ch chan<- *pbapiv1.WireGuardNotification, serverID int, serverTag string) (WatchCancelFunc, error) {
	m.rwMux.Lock()
	defer m.rwMux.Unlock()
	_, ok := m.configWatchers[serverTag]
	if ok {
		return nil, ErrAlreadyWatching
	}
	m.configWatchers[serverTag] = configWatcher{ch}
	err := m.increaseRepCount(ctx, serverID)
	if err != nil {
		m.logger.Errorf("Increasing replica count for serverID=%d: %v", serverID, err)
	}

	return func() {
		m.rwMux.Lock()
		delete(m.configWatchers, serverTag)
		err := m.pg.DecreaseReplicaCount(ctx, serverID)
		if err != nil {
			m.logger.Errorf("Decreasing replica count for serverID=%d: %v", serverID, err)
		}
		m.rwMux.Unlock()
	}, nil
}

func (m *postgresBasedVCM) WatchDeviceSessionUpdates(ctx context.Context) <-chan *pbapiv2.SubscribeForSessionsUpdatesResponse {
	if m.sessionsUpdatesWatcher == nil {
		m.sessionsUpdatesWatcher = make(chan *pbapiv2.SubscribeForSessionsUpdatesResponse)
	}

	return m.sessionsUpdatesWatcher
}

func (m *postgresBasedVCM) increaseRepCount(ctx context.Context, serverID int) error {
	if m.kubernetesClientset == nil {
		// do not increase replica count if there is
		// already one(non kubernetes deployment only)
		count, err := m.pg.GetReplicaCount(ctx, serverID)
		if err != nil {
			return err
		}
		if count == orchestratorNonKubernetesReplicaNum {
			return nil
		}
	}

	return m.pg.IncreaseReplicaCount(ctx, serverID)
}

func (m *postgresBasedVCM) QueryUsedAddrsForSubnet(ctx context.Context, subnet netip.Prefix) ([]netip.Addr, error) {
	if !ip.IsSubnetAddr(subnet) {
		return nil, errors.New("not a subnet address")
	}

	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	prefixes, err := m.sqlcQ.GetWireGuardIfaceAllAddrs(queryCtx)
	if err != nil {
		e := fmt.Errorf("%w: GetWireGuardIfaceAllAddrs: %w", pg.ErrQueryFailed, err)
		return nil, m.handleError(e)
	}

	var addrs []netip.Addr
	for _, prefix := range prefixes {
		if subnet.Contains(prefix.Addr()) {
			addrs = append(addrs, prefix.Addr())
		}
	}
	return addrs, nil
}

func (m *postgresBasedVCM) NotifyWGAddPeer(serverTag string, pubKey, presharedKey string, allowedIPs []string) {
	notification := &pbapiv1.WireGuardNotification{
		Data: &pbapiv1.WireGuardNotification_PeerNotification{
			PeerNotification: &pbapiv1.WireGuardPeerNotification{
				Operation: pbapiv1.WireGuardPeerNotification_ADD_PEER,
				Peer: &pbapiv1.WireGuardPeer{
					PublicKey:    pubKey,
					PresharedKey: presharedKey,
					AllowedIps:   allowedIPs,
				},
			},
		},
	}
	m.notifyWGConfigChanges(serverTag, notification)
}

func (m *postgresBasedVCM) NotifyWGDelPeerPubKey(serverTag string, pubKey string) {
	notification := &pbapiv1.WireGuardNotification{
		Data: &pbapiv1.WireGuardNotification_PeerNotification{
			PeerNotification: &pbapiv1.WireGuardPeerNotification{
				Operation: pbapiv1.WireGuardPeerNotification_DEL_PEER,
				Peer: &pbapiv1.WireGuardPeer{
					PublicKey: pubKey,
				},
			},
		},
	}
	m.notifyWGConfigChanges(serverTag, notification)
}

func (m *postgresBasedVCM) notifyWGServerConfigChange(serverTag string, wgConfig *pbapiv1.WireGuard) {
	notification := &pbapiv1.WireGuardNotification{
		Data: &pbapiv1.WireGuardNotification_ConfigNotification{
			ConfigNotification: wgConfig,
		},
	}
	m.notifyWGConfigChanges(serverTag, notification)
}

func (m *postgresBasedVCM) notifyWGConfigChanges(serverTag string, notification *pbapiv1.WireGuardNotification) {
	m.rwMux.RLock()
	watcher, ok := m.configWatchers[serverTag]
	m.rwMux.RUnlock()
	if !ok {
		m.logger.Debug("Notification was NOT sent, because no server watcher found.")
		return
	}

	select {
	case watcher.updates <- notification:
		m.logger.Debug("Notification was sent.")
	case <-time.After(5 * time.Second):
		m.logger.Warn("Notification was NOT sent, because channel is not listening.")
	}
}

func (m *postgresBasedVCM) notifyHealthcheckAddSession(session string) {
	notification := &pbapiv2.SubscribeForSessionsUpdatesResponse{
		Operation: pbapiv2.SubscribeForSessionsUpdatesResponse_OPERATION_ADD_SESSION,
		Session:   session,
	}
	m.notifyDeviceSessionChanges(notification)
}

func (m *postgresBasedVCM) notifyHealthcheckDeleteSession(session string) {
	notification := &pbapiv2.SubscribeForSessionsUpdatesResponse{
		Operation: pbapiv2.SubscribeForSessionsUpdatesResponse_OPERATION_DELETE_SESSION,
		Session:   session,
	}
	m.notifyDeviceSessionChanges(notification)
}

func (m *postgresBasedVCM) notifyDeviceSessionChanges(notification *pbapiv2.SubscribeForSessionsUpdatesResponse) {
	select {
	case m.sessionsUpdatesWatcher <- notification:
		m.logger.Debug("Notification was sent to healthcheck service.")
	case <-time.After(5 * time.Second):
		m.logger.Warn("Notification was NOT sent to healthcheck service, because channel is not listening.")
	}
}

func (m *postgresBasedVCM) getServerSideAllowedIP(serverAddresses, deviceAddresses []string) (netip.Prefix, error) {
	for _, serverAddress := range serverAddresses {
		serverNet, err := netip.ParsePrefix(serverAddress)
		if err != nil {
			m.logger.
				WithError(err).
				WithField("serverAddress", serverAddress).
				Warn("Could not parse server's address")
			continue
		}
		for _, deviceAddress := range deviceAddresses {
			deviceNet, err := netip.ParsePrefix(deviceAddress)
			if err != nil {
				m.logger.
					WithError(err).
					WithField("deviceAddress", deviceAddress).
					Warn("Could not parse device's address")
				continue
			}
			deviceAddr := deviceNet.Addr()
			if serverNet.Contains(deviceAddr) {
				return netip.PrefixFrom(deviceAddr, deviceAddr.BitLen()), nil
			}
		}
	}
	return netip.Prefix{}, fmt.Errorf("%w: could not find IP address of device's interface that is in the same network as one of server's IP addresses", ErrInvalidData)
}

// handleError translates error from postgres driver to general errors defined in VCM package.
func (m *postgresBasedVCM) handleError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pg.ErrQueryFailed) {
		m.logger.WithError(err).Error("Database error")
		return ErrDriverMisbehave
	}
	if errors.Is(err, pg.ErrNotFound) {
		m.logger.WithError(err).Error("Not found")
		return ErrNotFound
	}
	m.logger.WithError(err).Error("Unexpected response from database driver")
	return err
}

func wireguardConfigToProto(cfg *api.ServerWireGuardInterface, adjacencies []*api.WireGuardPeer) (*pbapiv1.WireGuard, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	var wgPeers []*pbapiv1.WireGuardPeer
	for _, p := range adjacencies {
		wgPeers = append(wgPeers, &pbapiv1.WireGuardPeer{
			PublicKey:           p.Config.PublicKey,
			PresharedKey:        p.Config.PresharedKey,
			AllowedIps:          p.Config.AllowedIPs,
			Endpoint:            p.Config.Endpoint,
			PersistentKeepalive: p.Config.PersistentKeepalive,
		})
	}

	wg := pbapiv1.WireGuard{
		Name: cfg.Name,
		Iface: &pbapiv1.WireGuardInterface{
			PrivateKey: cfg.PrivateKey,
			Addresses:  cfg.Addresses,
			ListenPort: cfg.ListenPort,
			Dns:        cfg.DNS,
			Mtu:        cfg.MTU,
		},
		Peers: wgPeers,
	}
	return &wg, nil
}

func IsHealthcheckAddrInAllowedIPs(clientSideAllowedIPs []netip.Prefix, healthcheckAddr *netip.Addr) bool {
	if healthcheckAddr == nil { // healthcheck service is not activated for adjacency, ignore validation
		return true
	}
	for _, aip := range clientSideAllowedIPs {
		if aip.Contains(*healthcheckAddr) {
			return true
		}
	}

	return false
}

// EnsureHealthcheckAddrInAllowedIPs adds the healthcheck address to the allowed IPs list if not already contained.
// Returns the updated allowed IPs list and a boolean indicating if the list was modified.
func EnsureHealthcheckAddrInAllowedIPs(clientSideAllowedIPs []netip.Prefix, healthcheckAddr *netip.Addr) ([]netip.Prefix, bool) {
	if IsHealthcheckAddrInAllowedIPs(clientSideAllowedIPs, healthcheckAddr) {
		return clientSideAllowedIPs, false
	}

	bits := 32
	if healthcheckAddr.Is6() {
		bits = 128
	}
	hcPrefix := netip.PrefixFrom(*healthcheckAddr, bits)

	return append(clientSideAllowedIPs, hcPrefix), true
}

// RemoveHealthcheckAddrFromAllowedIPs removes the healthcheck address from the allowed IPs list.
// Returns the updated allowed IPs list and a boolean indicating if the list was modified.
func RemoveHealthcheckAddrFromAllowedIPs(clientSideAllowedIPs []netip.Prefix, healthcheckAddr *netip.Addr) ([]netip.Prefix, bool) {
	if healthcheckAddr == nil {
		return clientSideAllowedIPs, false
	}

	bits := 32
	if healthcheckAddr.Is6() {
		bits = 128
	}
	hcPrefix := netip.PrefixFrom(*healthcheckAddr, bits)

	modified := false
	var result []netip.Prefix
	for _, prefix := range clientSideAllowedIPs {
		if prefix != hcPrefix {
			result = append(result, prefix)
		} else {
			modified = true
		}
	}

	return result, modified
}

// GetWireGuardServerAdjacencies returns a list of adjacencies (clients) for a server with given id.
func GetWireGuardServerAdjacencies(ctx context.Context, conn *pgxpool.Pool, encryptionKey []byte, serverID int) ([]*pg.WireGuardAdjacency, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	var peers []*pg.WireGuardAdjacency
	err := db.WithTx(queryCtx, conn, func(qsqlc *sqlc.Queries) error {
		dbPeers, err := qsqlc.GetServerWireGuardAdjacencies(queryCtx, serverID)
		if err != nil {
			return fmt.Errorf("GetServerWireGuardAdjacencies: %w", err)
		}

		for _, dbPeer := range dbPeers {
			psk, err := aesgcm.OpenString(encryptionKey, dbPeer.PresharedKeyEncrypted)
			if err != nil {
				return fmt.Errorf("failed to decrypt preshared key: %w", err)
			}

			peer := &pg.WireGuardAdjacency{
				ID:                  dbPeer.ID,
				DeviceID:            dbPeer.DeviceID,
				PresharedKey:        psk,
				ListenPort:          dbPeer.ListenPort,
				PublicKey:           dbPeer.PublicKey,
				PersistentKeepalive: -1,
			}

			allowedIPs := dbPeer.ServerSideAllowedIps
			for _, ip := range allowedIPs {
				peer.AllowedIPs = append(peer.AllowedIPs, ip.String())
			}

			otherSideIPs := dbPeer.ClientSideAllowedIps
			for _, ip := range otherSideIPs {
				peer.OtherSideAllowedIPs = append(peer.OtherSideAllowedIPs, ip.String())
			}

			peers = append(peers, peer)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", pg.ErrQueryFailed, err)
	}
	return peers, nil
}

func getDeviceTemplateByUserID(ctx context.Context, sqlcQ *sqlc.Queries, userID int) (*pg.DeviceTemplate, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	dt, err := sqlcQ.GetDeviceTemplateByUserID(queryCtx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: GetDeviceTemplateByUserID: %w", pg.ErrQueryFailed, err)
	}

	addressPools, err := sqlcQ.GetDeviceTemplateAddressPools(queryCtx, dt.ID)

	if err != nil {
		return nil, fmt.Errorf("%w: GetDeviceTemplateAddressPools: %w", pg.ErrQueryFailed, err)
	}

	dnss, err := sqlcQ.GetDeviceTemplateWireGuardIfaceDNSs(queryCtx, dt.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetDeviceTemplateWireGuardIfaceDNSs: %w", pg.ErrQueryFailed, err)
	}

	var pAddressPools []*sqlc.AddressPool
	for _, p := range addressPools {
		pAddressPools = append(pAddressPools, &p)
	}

	result := &pg.DeviceTemplate{
		ID:            dt.ID,
		InterfaceName: dt.InterfaceName,
		ListenPort:    dt.ListenPort,
		MTU:           dt.Mtu,
		AddressPools:  pAddressPools,
		DNS:           dnss,
	}
	return result, nil
}

func getDeviceWireGuardConfig(ctx context.Context, sqlcQ *sqlc.Queries, encryptionKey []byte, deviceID int) (*pg.WireGuardConfig, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	device, err := sqlcQ.GetDeviceByID(queryCtx, deviceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: GetDeviceByID: %w", pg.ErrQueryFailed, err)
	}

	dt, err := sqlcQ.GetDeviceTemplateByID(queryCtx, device.DeviceTemplateID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: GetDeviceTemplateByID: %w", pg.ErrQueryFailed, err)
	}

	addrs, err := sqlcQ.GetDeviceWireGuardIfaceAddrs(queryCtx, device.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetDeviceWireGuardIfaceAddrs: %w", pg.ErrQueryFailed, err)
	}
	var addresses []string
	for _, a := range addrs {
		addresses = append(addresses, a.String())
	}

	dnss, err := sqlcQ.GetDeviceTemplateWireGuardIfaceDNSs(queryCtx, device.DeviceTemplateID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetDeviceTemplateWireGuardIfaceDNSs: %w", pg.ErrQueryFailed, err)
	}

	priv, err := aesgcm.OpenString(encryptionKey, device.PrivateKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key: %w", err)
	}

	wgc := &pg.WireGuardConfig{
		ID:                  device.ID,
		InterfaceName:       dt.InterfaceName,
		PrivateKey:          priv,
		PublicKey:           device.PublicKey,
		ListenPort:          dt.ListenPort,
		MTU:                 dt.Mtu,
		Addresses:           addresses,
		DNS:                 dnss,
		PersistentKeepalive: -1,
	}
	return wgc, nil
}

// GetWireGuardServerConfig returns WireGuard VPN configuration for a server with given id.
func GetWireGuardServerConfig(ctx context.Context, sqlcQ *sqlc.Queries, encryptionKey []byte, serverID int) (*pg.WireGuardConfig, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	serverCfg, err := sqlcQ.GetServerWireGuardConfig(queryCtx, serverID)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: GetServerWireGuardConfig: %w", pg.ErrQueryFailed, err)
	}

	addrs, err := sqlcQ.GetServerWireGuardIfaceAddrs(queryCtx, serverCfg.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetServerWireGuardIfaceAddrs: %w", pg.ErrQueryFailed, err)
	}
	var addresses []string
	for _, a := range addrs {
		addresses = append(addresses, a.String())
	}

	dnss, err := sqlcQ.GetServerWireGuardIfaceDNSs(queryCtx, serverCfg.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: GetServerWireGuardIfaceDNSs: %w", pg.ErrQueryFailed, err)
	}

	priv, err := aesgcm.OpenString(encryptionKey, serverCfg.PrivateKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key: %w", err)
	}

	wgc := &pg.WireGuardConfig{
		ID:                  serverCfg.ID,
		InterfaceName:       serverCfg.InterfaceName,
		PrivateKey:          priv,
		PublicKey:           serverCfg.PublicKey,
		ListenPort:          serverCfg.ListenPort,
		MTU:                 serverCfg.Mtu,
		Addresses:           addresses,
		DNS:                 dnss,
		PersistentKeepalive: serverCfg.PersistentKeepalive,
	}
	return wgc, nil
}

// GetWireGuardDeviceAdjacencies returns a list of adjacencies (servers) for a device with given id.
func GetWireGuardDeviceAdjacencies(ctx context.Context, conn *pgxpool.Pool, encryptionKey []byte, deviceID int) ([]*pg.WireGuardAdjacency, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	var peers []*pg.WireGuardAdjacency
	err := db.WithTx(queryCtx, conn, func(qsqlc *sqlc.Queries) error {
		dbPeers, err := qsqlc.GetDeviceWireGuardAdjacencies(queryCtx, deviceID)
		if err != nil {
			return fmt.Errorf("GetDeviceWireGuardAdjacencies: %w", err)
		}

		for _, dbPeer := range dbPeers {
			psk, err := aesgcm.OpenString(encryptionKey, dbPeer.PresharedKeyEncrypted)
			if err != nil {
				return fmt.Errorf("failed to decrypt preshared key: %w", err)
			}

			peer := &pg.WireGuardAdjacency{
				ID:                  dbPeer.ID,
				PresharedKey:        psk,
				PersistentKeepalive: dbPeer.PersistentKeepalive,
				ListenPort:          dbPeer.ListenPort,
				PublicKey:           dbPeer.PublicKey,
				DeviceID:            deviceID,
				Server: &pg.Server{
					ID:                 dbPeer.Serverid,
					Name:               dbPeer.Servername,
					Endpoint:           dbPeer.Serverendpoint,
					Description:        dbPeer.Serverdescription,
					HealthCheckAddress: dbPeer.Serverhealthcheckaddress,
				},
			}

			allowedIPs := dbPeer.ClientSideAllowedIps
			for _, ip := range allowedIPs {
				peer.AllowedIPs = append(peer.AllowedIPs, ip.String())
			}

			otherSideIPs := dbPeer.ServerSideAllowedIps
			for _, ip := range otherSideIPs {
				peer.OtherSideAllowedIPs = append(peer.OtherSideAllowedIPs, ip.String())
			}

			peers = append(peers, peer)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", pg.ErrQueryFailed, err)
	}
	return peers, nil
}

func InsertServerWithWireGuardConfig(
	ctx context.Context,
	conn *pgxpool.Pool,
	encryptionKey []byte,
	s *pg.Server,
	c *pg.WireGuardConfig,
) (int, error) {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	serverID := 0
	err := db.WithTx(queryCtx, conn, func(qsqlc *sqlc.Queries) error {
		server, err := qsqlc.InsertServer(queryCtx, sqlc.InsertServerParams{
			Name:               s.Name,
			Endpoint:           s.Endpoint,
			Description:        s.Description,
			HealthcheckAddress: s.HealthCheckAddress,
		})
		if err != nil {
			return fmt.Errorf("InsertServer: %w", err)
		}

		serverID = server.ID

		privKeyEnc, err := aesgcm.Seal(encryptionKey, c.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt private key: %w", err)
		}
		serverCfg, err := qsqlc.InsertServerWireGuardConfig(queryCtx, sqlc.InsertServerWireGuardConfigParams{
			ServerID:            server.ID,
			InterfaceName:       c.InterfaceName,
			PrivateKeyEncrypted: privKeyEnc,
			PublicKey:           c.PublicKey,
			ListenPort:          db.IntToPtr(c.ListenPort),
			Mtu:                 db.IntToPtr(c.MTU),
			PersistentKeepalive: db.IntToPtr(c.PersistentKeepalive),
		})
		if err != nil {
			return fmt.Errorf("InsertServerWireGuardConfig: %w", err)
		}

		for _, addr := range c.Addresses {
			parsedPrefix, err := netip.ParsePrefix(addr)
			if err != nil {
				return fmt.Errorf("parse address '%s': %w", addr, err)
			}
			_, err = qsqlc.InsertServerWireGuardIfaceAddr(queryCtx, sqlc.InsertServerWireGuardIfaceAddrParams{
				ServerConfigID: serverCfg.ID,
				Addr:           parsedPrefix,
			})
			if err != nil {
				return fmt.Errorf("InsertServerWireGuardIfaceAddr: %w", err)
			}
		}

		for _, dns := range c.DNS {
			_, err := qsqlc.InsertServerWireGuardIfaceDNS(queryCtx, sqlc.InsertServerWireGuardIfaceDNSParams{
				ServerConfigID: serverCfg.ID,
				Dns:            dns,
			})
			if err != nil {
				return fmt.Errorf("InsertServerWireGuardIfaceDNS: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("%w: %v", pg.ErrQueryFailed, err)
	}

	return serverID, nil
}

// deleteTemplateDevicesSessions clears sessions for all devices belonging to a template
func deleteTemplateDevicesSessions(ctx context.Context, q *sqlc.Queries, templateID int) error {
	queryCtx, cancel := context.WithTimeout(ctx, pg.QueryTimeout)
	defer cancel()

	_, err := q.InvalidateTemplateDeviceSessions(queryCtx, templateID)
	if err != nil {
		return fmt.Errorf("%w: InvalidateTemplateDeviceSessions: %v", pg.ErrQueryFailed, err)
	}
	return nil
}
