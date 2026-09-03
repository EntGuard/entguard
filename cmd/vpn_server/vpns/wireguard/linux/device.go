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

package linux

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	pbapi "github.com/entguard/entguard/proto/v1"
)

// DeviceConfig represents a WireGuard device configuration builder.
type DeviceConfig struct {
	privateKey   *wgtypes.Key
	listenPort   *int
	replacePeers bool
	peers        []wgtypes.PeerConfig
	lastError    error
}

// NewDeviceConfig returns a new DeviceConfig.
func NewDeviceConfig(pk, port string) (*DeviceConfig, error) {
	wgcPrivateKey, err := wgtypes.ParseKey(pk)
	if err != nil {
		return nil, err
	}

	var wgcListenPort *int
	if lp := port; lp != "" {
		port, err := strconv.Atoi(lp)
		if err != nil {
			return nil, err
		}
		wgcListenPort = &port
	}

	dc := &DeviceConfig{
		privateKey: &wgcPrivateKey,
		listenPort: wgcListenPort,
	}

	return dc, nil
}

func (dc *DeviceConfig) Create() *DeviceConfig {
	dc.replacePeers = true
	dc.peers = nil
	dc.lastError = nil
	return dc
}

func (dc *DeviceConfig) Update() *DeviceConfig {
	dc.replacePeers = false
	dc.peers = nil
	dc.lastError = nil
	return dc
}

// AddPeerE adds a peer to the device configuration and returns error.
func (dc *DeviceConfig) AddPeerE(p *pbapi.WireGuardPeer) error {
	peer, err := newPeer(p)
	if err != nil {
		return fmt.Errorf("bad peer configuration: %v", err)
	}
	peer.Remove = false
	dc.peers = append(dc.peers, peer)
	return nil
}

// AddPeer adds a peer to the device configuration and saves error.
func (dc *DeviceConfig) AddPeer(p *pbapi.WireGuardPeer) *DeviceConfig {
	if err := dc.AddPeerE(p); err != nil {
		dc.lastError = err
	}
	return dc
}

// DelPeerE adds a peer (and marks it for removal) to the device configuration and returns error.
func (dc *DeviceConfig) DelPeerE(p *pbapi.WireGuardPeer) error {
	peer, err := newPeer(p)
	if err != nil {
		return fmt.Errorf("bad peer configuration: %v", err)
	}
	peer.Remove = true
	dc.peers = append(dc.peers, peer)
	return nil
}

// DelPeer adds a peer (and marks it for removal) to the device configuration and saves error.
func (dc *DeviceConfig) DelPeer(p *pbapi.WireGuardPeer) *DeviceConfig {
	if err := dc.DelPeerE(p); err != nil {
		dc.lastError = err
	}
	return dc
}

// Done returns device configuration.
func (dc *DeviceConfig) Done() (*wgtypes.Config, error) {
	if dc.lastError != nil {
		return nil, dc.lastError
	}

	cfg := &wgtypes.Config{
		PrivateKey:   dc.privateKey,
		ListenPort:   dc.listenPort,
		ReplacePeers: dc.replacePeers,
		Peers:        dc.peers,
	}

	return cfg, nil
}

func newPeer(p *pbapi.WireGuardPeer) (wgtypes.PeerConfig, error) {
	var peer wgtypes.PeerConfig

	if p.GetPublicKey() == "" {
		return peer, errors.New("missing public key")
	}
	peerPublicKey, err := wgtypes.ParseKey(p.PublicKey)
	if err != nil {
		return peer, fmt.Errorf("bad public key %q", p.PublicKey)
	}

	peer = wgtypes.PeerConfig{
		PublicKey:         peerPublicKey,
		ReplaceAllowedIPs: true,
	}

	if p.GetPresharedKey() != "" {
		peerPresharedKey, err := wgtypes.ParseKey(p.PresharedKey)
		if err != nil {
			return peer, fmt.Errorf("bad preshared key %q", p.PresharedKey)
		}
		peer.PresharedKey = &peerPresharedKey
	}

	if p.GetEndpoint() != "" {
		peerEndpoint, err := net.ResolveUDPAddr("udp", p.Endpoint)
		if err != nil {
			return peer, fmt.Errorf("bad endpoint %q", p.Endpoint)
		}
		peer.Endpoint = peerEndpoint
	}

	if p.GetPersistentKeepalive() != "" {
		peerPersistentKeepalive, err := strconv.Atoi(p.PersistentKeepalive)
		if err != nil {
			return peer, fmt.Errorf("bad persistent keepalive interval %q", p.PersistentKeepalive)
		}

		peerPersistentKeepaliveInterval := time.Duration(peerPersistentKeepalive) * time.Second
		peer.PersistentKeepaliveInterval = &peerPersistentKeepaliveInterval
	}

	if len(p.GetAllowedIps()) != 0 {
		var allowedIPs []net.IPNet

		for _, aip := range p.AllowedIps {
			_, ipnet, err := net.ParseCIDR(aip)
			if err != nil {
				return peer, fmt.Errorf("bad IP address found in the allowed IPs list: %q", aip)
			}
			allowedIPs = append(allowedIPs, *ipnet)
		}

		peer.AllowedIPs = allowedIPs
	}

	return peer, nil
}
