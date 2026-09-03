/*
 * Copyright 2024 PANTHEON.tech s.r.o.
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

package wireguard

import (
	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/cmd/vpn_server/vpns/wireguard/linux"
	"github.com/entguard/entguard/cmd/vpn_server/vpns/wireguard/vpp"
	pbapi "github.com/entguard/entguard/proto/v1"
)

// Client handles wireguard tunnel on server side. That includes not only wireguard-specific configuration
// like peers, but also some hidden close related configuration (i.e. wg interface creation using netlink in linux
// client implementation) that can't be statically created at deployment time (day0 configuration applied by egvpn tool)
type Client interface {
	// UpdateWireGuardInterface updates existing WireGuard interface (it does not update peers)
	UpdateWireGuardInterface(newWGC *pbapi.WireGuard) error
	// AddWireGuard creates local WireGuard tunnel end (properly configured Wireguard link, Wireguard peers, routes,....).
	AddWireGuard(wgc *pbapi.WireGuard) error
	// AddPeers adds the list of peers to the WireGuard interface.
	// It also adds steering routes for allowed IPs of given peers.
	AddPeers(peers ...*pbapi.WireGuardPeer) error
	// DelPeers deletes the list of peers from the WireGuard interface.
	// It also removes steering routes for allowed IPs of given peers.
	DelPeers(peers ...*pbapi.WireGuardPeer) error
	// Close does cleanup and closes the client. Do not use the client after close.
	Close()
}

func NewClient(log *log.Entry, useVPP bool) (Client, error) {
	if useVPP {
		log.Debug("Creating VPP WireGuard client")
		return vpp.NewClient(log)
	}
	log.Debug("Creating Linux WireGuard client")
	return linux.NewClient(log)
}
