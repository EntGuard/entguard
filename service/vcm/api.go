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
	"net/netip"

	"k8s.io/client-go/kubernetes"

	sqlc "github.com/entguard/entguard/db"
	pbapiv1 "github.com/entguard/entguard/proto/v1"
	pbapiv2 "github.com/entguard/entguard/proto/v2"
	"github.com/entguard/entguard/service/api/v1"
	pg "github.com/entguard/entguard/service/vcm/drivers/postgres"
)

// WatchCancelFunc allows to unsubscribe from notifications.
type WatchCancelFunc func()

// VPNConfigManager defines available methods to manage VPN configuration and to manage server certificates.
type VPNConfigManager interface {
	GetAllServers(ctx context.Context) (*api.ServerList, error)
	HandlePeerUpdates(ctx context.Context, fails chan error)
	HandleServerUpdates(ctx context.Context, fails chan error)
	HandleSessionsUpdatesForHealthcheckService(ctx context.Context, fails chan error)
	GetServerByID(ctx context.Context, id int) (*api.Server, error)
	GetServerByName(ctx context.Context, name string) (*api.Server, error)
	CheckServerExists(ctx context.Context, id int) (bool, error)
	CreateServer(ctx context.Context, s *api.ServerWithVPNConfig) error
	UpdateServer(ctx context.Context, id int, s *api.ServerUpdate) (*api.Server, error)
	DeleteServer(ctx context.Context, id int) error

	// Methods GetCACertificate, VerifyServerTag & GetCertForServer are used to manage EG-S certificates.
	// GetCertForServer is used by REST service to provide certificates to the EG-S.
	// GetCACertificate is used by RPC server service to verify beforementioned certificates.
	// VerifyServerTag is used to verify tag for server with specified name.
	// These methods probably should not be the part of VCM.
	// When the better alternative is found they should be moved somewhere else.
	GetCACertificate(ctx context.Context) *x509.Certificate
	GetCertForServer(ctx context.Context, serverName string) (cert []byte, key []byte, e error)
	VerifyServerTag(ctx context.Context, unverifiedTag, serverName string) bool

	CheckDeviceTemplateExistsByUserID(ctx context.Context, userID int) (bool, error)
	GetDeviceTemplateByUserID(ctx context.Context, userID int) (*api.DeviceTemplate, error)
	CreateDeviceTemplate(ctx context.Context, userID int, apiDT *api.DeviceTemplate) (int, error)
	UpdateDeviceTemplateByUserID(ctx context.Context, userID int, apiDT *api.DeviceTemplate) error
	DeleteDeviceTemplateByUserID(ctx context.Context, userID int) error

	GetDeviceWireGuardConfig(ctx context.Context, deviceID int) (*api.ServerWireGuardInterface, error)

	GetWireGuardServerConfig(ctx context.Context, serverID int) (*api.ServerWireGuardInterface, error)
	UpdateWireGuardServerConfig(ctx context.Context, serverID int, c *api.ServerWireGuardInterface) error

	GetWireGuardServerAdjacencies(ctx context.Context, serverID int) ([]*api.WireGuardPeer, error)
	GetWireGuardDeviceAdjacencies(ctx context.Context, deviceID int) ([]*api.WireGuardPeer, error)
	GetUserDeviceAdjacencies(ctx context.Context, userID int) ([]*api.AdjacencyExtended, error)
	CreateAdjacency(ctx context.Context, serverID, deviceID int, wgAdj *api.WireGuardAdjacency) error
	UpdateAdjacency(ctx context.Context, adjacencyID int, serverID, deviceID int, wgAdj *api.WireGuardAdjacency) error
	DeleteAdjacency(ctx context.Context, adjacencyID int) error

	GetUserAdjacencyTemplates(ctx context.Context, userID int) ([]*api.AdjacencyTemplateExtended, error)
	CreateAdjacencyTemplate(ctx context.Context, server *api.Server, userID int, wgAdj *api.WireGuardAdjacencyTemplate) error
	UpdateAdjacencyTemplate(ctx context.Context, adjacencyTemplateID int, server *api.Server, userID int, wgAdj *api.WireGuardAdjacencyTemplate) error
	DeleteAdjacencyTemplate(ctx context.Context, adjacencyTemplateID int) error

	ResyncDeviceAdjacencies(ctx context.Context, userID int) (*api.DeviceAdjacencyResyncResponse, error)

	QueryUsedAddrsForSubnet(ctx context.Context, subnet netip.Prefix) ([]netip.Addr, error)

	NotifyWGAddPeer(serverTag string, pubKey, presharedKey string, allowedIPs []string)
	NotifyWGDelPeerPubKey(serverTag string, pubKey string)

	// WatchWireGuardConfig allows to subscribe for configuration updates of WireGuard VPN server.
	WatchWireGuardConfig(ctx context.Context, channel chan<- *pbapiv1.WireGuardNotification, serverID int, serverTag string) (WatchCancelFunc, error)

	// WatchDeviceSessionUpdates allows to subscribe for device session updates for healthcheck service.
	WatchDeviceSessionUpdates(ctx context.Context) <-chan *pbapiv2.SubscribeForSessionsUpdatesResponse

	SetKubernetesClientset(ctx context.Context, clients *kubernetes.Clientset)
	AllReplicasRunning(ctx context.Context, server *api.Server) (bool, error)

	DeleteInvalidSessionsByDeviceTemplate(ctx context.Context, templateID int, old, new *api.DeviceTemplate) error
	DeleteInvalidSessionsByAdjacency(ctx context.Context, old, new *sqlc.Adjacency) error
	DeleteInvalidSessionsByServer(ctx context.Context, old, new *pg.Server) error
	DeleteInvalidSessionsByServerWGConfig(ctx context.Context, serverID int, old, new *pg.WireGuardConfig) error
}
