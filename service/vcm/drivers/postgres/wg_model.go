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

package postgres

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/crypto/aesgcm"
	"github.com/entguard/entguard/pkg/db"
	"github.com/entguard/entguard/pkg/device"
	"github.com/entguard/entguard/service/api/v1"
)

const (
	keyLength              = 32
	maxInterfaceNameLength = 15
	maxListenPort          = 65535
	minMTU                 = 576
	maxMTU                 = 65535
	maxPersistentKeepalive = 65535
)

type WireGuardConfig struct {
	// ID is server WG config ID or device ID (not device template ID) // it is unused anyway
	ID                  int
	InterfaceName       string
	PrivateKey          string
	PublicKey           string
	Addresses           []string
	ListenPort          int
	DNS                 []string
	MTU                 int
	PersistentKeepalive int
}

type DeviceTemplate struct {
	ID            int
	InterfaceName string
	AddressPools  []*sqlc.AddressPool
	ListenPort    int
	DNS           []string
	MTU           int
}

type WireGuardAdjacencyConfig struct {
	PresharedKey         string
	ServerSideAllowedIPs []netip.Prefix
	ClientSideAllowedIPs []netip.Prefix
}

type WireGuardAdjacencyTemplateConfig struct {
	UsePresharedKey      bool
	ClientSideAllowedIPs []netip.Prefix
}

type WireGuardAdjacency struct {
	ID                  int
	AllowedIPs          []string
	OtherSideAllowedIPs []string
	PresharedKey        string
	PersistentKeepalive int
	DeviceID            int
	Server              *Server
	ListenPort          int
	PublicKey           string
}

func NewWireGuardConfigFromAPI(apiWGIface *api.ServerWireGuardInterface) (*WireGuardConfig, error) {
	if apiWGIface.Name == "" {
		return nil, errors.New("name of WireGuard interface cannot be empty")
	}
	if apiWGIface.PrivateKey == "" {
		return nil, errors.New("missing private key of WireGuard interface")
	}
	if apiWGIface.PublicKey == "" {
		return nil, errors.New("missing public key of WireGuard interface")
	}
	if len(apiWGIface.Addresses) == 0 {
		return nil, errors.New("at least one address in the Addresses list must be defined")
	}

	var err error

	if err = validateInterfaceName(apiWGIface.Name); err != nil {
		return nil, fmt.Errorf("invalid interface name: %v", err)
	}
	if err = ValidateKeyBase64(apiWGIface.PrivateKey); err != nil {
		return nil, fmt.Errorf("invalid private key: %v", err)
	}
	if err = ValidateKeyBase64(apiWGIface.PublicKey); err != nil {
		return nil, fmt.Errorf("invalid public key: %v", err)
	}

	listenPort := -1
	if apiWGIface.ListenPort != "" {
		listenPort, err = strconv.Atoi(apiWGIface.ListenPort)
		if err != nil {
			return nil, fmt.Errorf("could not read port value: %v", err)
		}
		if listenPort < 0 || listenPort > maxListenPort {
			return nil, fmt.Errorf("invalid port value: %d", listenPort)
		}
	}

	mtu := -1
	if apiWGIface.MTU != "" {
		mtu, err = strconv.Atoi(apiWGIface.MTU)
		if err != nil {
			return nil, fmt.Errorf("could not read MTU value: %v", err)
		}
		if mtu < minMTU || mtu > maxMTU {
			return nil, fmt.Errorf("invalid MTU value: %d", mtu)
		}
	}

	persistentKeepalive := -1
	if apiWGIface.PersistentKeepalive != "" {
		persistentKeepalive, err = strconv.Atoi(apiWGIface.PersistentKeepalive)
		if err != nil {
			return nil, fmt.Errorf("could not read persisten keepalive value: %v", err)
		}
		if persistentKeepalive < 0 || persistentKeepalive > maxPersistentKeepalive {
			return nil, fmt.Errorf("invalid persistent keepalive value: %d", persistentKeepalive)
		}
	}

	addrs := make([]string, 0, len(apiWGIface.Addresses))
	for _, addr := range apiWGIface.Addresses {
		err := validateIPCidr(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address in the Addresses list: %v", err)
		}
		addrs = append(addrs, addr)
	}

	dnss := make([]string, len(apiWGIface.DNS))
	copy(dnss, apiWGIface.DNS)

	wgc := &WireGuardConfig{
		InterfaceName:       apiWGIface.Name,
		PrivateKey:          apiWGIface.PrivateKey,
		PublicKey:           apiWGIface.PublicKey,
		ListenPort:          listenPort,
		Addresses:           addrs,
		MTU:                 mtu,
		DNS:                 dnss,
		PersistentKeepalive: persistentKeepalive,
	}

	return wgc, nil
}

func NewDeviceTemplateFromAPI(apiDT *api.DeviceTemplate) (*DeviceTemplate, error) {
	if apiDT.InterfaceName == "" {
		return nil, errors.New("name of WireGuard interface cannot be empty")
	}
	if len(apiDT.AddressPools) == 0 {
		return nil, errors.New("at least one address pool in the Address pools list must be defined")
	}

	var err error

	if err = validateInterfaceName(apiDT.InterfaceName); err != nil {
		return nil, fmt.Errorf("invalid interface name: %v", err)
	}

	listenPort := -1
	if apiDT.ListenPort != "" {
		listenPort, err = strconv.Atoi(apiDT.ListenPort)
		if err != nil {
			return nil, fmt.Errorf("could not read port value: %v", err)
		}
		if listenPort < 0 || listenPort > maxListenPort {
			return nil, fmt.Errorf("invalid port value: %d", listenPort)
		}
	}

	mtu := -1
	if apiDT.MTU != "" {
		mtu, err = strconv.Atoi(apiDT.MTU)
		if err != nil {
			return nil, fmt.Errorf("could not read MTU value: %v", err)
		}
		if mtu < minMTU || mtu > maxMTU {
			return nil, fmt.Errorf("invalid MTU value: %d", mtu)
		}
	}

	var addressPools []*sqlc.AddressPool
	for _, ap := range apiDT.AddressPools {
		addressPools = append(addressPools, &sqlc.AddressPool{
			ID: ap.ID,
		})
	}

	dnss := make([]string, len(apiDT.DNS))
	copy(dnss, apiDT.DNS)

	dt := &DeviceTemplate{
		InterfaceName: apiDT.InterfaceName,
		ListenPort:    listenPort,
		AddressPools:  addressPools,
		MTU:           mtu,
		DNS:           dnss,
	}

	return dt, nil
}

func NewWireGuardAdjacencyConfigFromAPI(apiWGPC *api.WireGuardAdjacency) (*WireGuardAdjacencyConfig, error) {
	if len(apiWGPC.AllowedIPs) == 0 {
		return nil, errors.New("at least one address in the Allowed IPs list must be defined")
	}

	if apiWGPC.ServerSide && len(apiWGPC.OtherSideAllowedIPs) == 0 {
		return nil, errors.New("at least one address in the Client side Allowed IPs list must be defined")
	}

	if apiWGPC.PresharedKey != "" {
		if err := ValidateKeyBase64(apiWGPC.PresharedKey); err != nil {
			return nil, fmt.Errorf("invalid Preshared Key: %v", err)
		}
	}

	// Allowed IPs are used for matching IP addresses of incoming and outgoing packets.
	// Therefore, we are interested in networks defined by these IPs.
	ips := make([]netip.Prefix, 0, len(apiWGPC.AllowedIPs))
	sameIPNets := make(map[netip.Prefix]struct{})
	for _, addr := range apiWGPC.AllowedIPs {
		ipNet, err := ParseIPNet(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address in the Allowed IPs list: %v", err)
		}
		_, inUse := sameIPNets[ipNet]
		if !inUse {
			ips = append(ips, ipNet)
			sameIPNets[ipNet] = struct{}{}
		}
	}

	oSIps := make([]netip.Prefix, 0, len(apiWGPC.OtherSideAllowedIPs))
	oSSameIPNets := make(map[netip.Prefix]struct{})
	for _, oSAddr := range apiWGPC.OtherSideAllowedIPs {
		ipNet, err := ParseIPNet(oSAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid address in the Other Side Allowed IPs list: %v", err)
		}
		_, inUse := oSSameIPNets[ipNet]
		if !inUse {
			oSIps = append(oSIps, ipNet)
			oSSameIPNets[ipNet] = struct{}{}
		}
	}

	wgpc := &WireGuardAdjacencyConfig{
		PresharedKey: apiWGPC.PresharedKey,
	}

	if apiWGPC.ServerSide {
		wgpc.ServerSideAllowedIPs = ips
		wgpc.ClientSideAllowedIPs = oSIps
	} else {
		wgpc.ClientSideAllowedIPs = ips
	}

	return wgpc, nil
}

func NewWireGuardAdjacencyTemplateConfigFromAPI(apiWGPC *api.WireGuardAdjacencyTemplate) (*WireGuardAdjacencyTemplateConfig, error) {
	if len(apiWGPC.ClientSideAllowedIPs) == 0 {
		return nil, errors.New("at least one address in the Client Side Allowed IPs list must be defined")
	}

	// Allowed IPs are used for matching IP addresses of incoming and outgoing packets.
	// Therefore, we are interested in networks defined by these IPs.
	ips := make([]netip.Prefix, 0, len(apiWGPC.ClientSideAllowedIPs))
	sameIPNets := make(map[netip.Prefix]struct{})
	for _, addr := range apiWGPC.ClientSideAllowedIPs {
		ipNet, err := ParseIPNet(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address in the Allowed IPs list: %v", err)
		}
		_, inUse := sameIPNets[ipNet]
		if !inUse {
			ips = append(ips, ipNet)
			sameIPNets[ipNet] = struct{}{}
		}
	}

	wgpc := &WireGuardAdjacencyTemplateConfig{
		ClientSideAllowedIPs: ips,
		UsePresharedKey:      apiWGPC.UsePresharedKey,
	}

	return wgpc, nil
}

// This is copy-pasted function `ConvertDBToAPIDevice` from package `user`.
// Using the original function would cause circular dependency.
func ConvertDBToAPIDevice(encryptionKey []byte, d sqlc.Device, addrs []netip.Prefix) (*api.Device, error) {
	priv, err := aesgcm.OpenString(encryptionKey, d.PrivateKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key: %w", err)
	}

	apiDevice := &api.Device{
		ID: d.ID,
		DeviceData: api.DeviceData{
			Description: d.Description,
			PrivateKey:  priv,
			PublicKey:   d.PublicKey,
		},
		ExternalDeviceID:  d.ExternalDeviceID,
		SessionID:         d.SessionID,
		LastTimeConnected: d.LastTimeConnected.Time,
		DeviceInformation: device.ParseInformation(db.StringFromPtr(d.DeviceInformation)),
	}
	for _, a := range addrs {
		apiDevice.Addresses = append(apiDevice.Addresses, a.String())
	}
	return apiDevice, nil
}

// This is copy-pasted function `ConvertDBToAPIPool` from package `user`.
// Using the original function would cause circular dependency.
func ConvertDBToAPIPool(p *sqlc.AddressPool) *api.AddressPool {
	return &api.AddressPool{
		ID:          p.ID,
		Name:        p.Name,
		StartAddr:   p.StartAddr.String(),
		EndAddr:     p.EndAddr.String(),
		NetMask:     p.NetMask,
		Description: p.Description,
	}
}

func (wgc *WireGuardConfig) ToAPI() *api.ServerWireGuardInterface {
	addrs := make([]string, len(wgc.Addresses))
	copy(addrs, wgc.Addresses)

	dnss := make([]string, len(wgc.DNS))
	copy(dnss, wgc.DNS)

	var listenPort, mtu, persistentKeepalive string
	if wgc.ListenPort != -1 {
		listenPort = strconv.Itoa(wgc.ListenPort)
	}
	if wgc.MTU != -1 {
		mtu = strconv.Itoa(wgc.MTU)
	}
	if wgc.PersistentKeepalive != -1 {
		persistentKeepalive = strconv.Itoa(wgc.PersistentKeepalive)
	}

	apiWGC := &api.ServerWireGuardInterface{
		Name:                wgc.InterfaceName,
		PrivateKey:          wgc.PrivateKey,
		PublicKey:           wgc.PublicKey,
		Addresses:           addrs,
		ListenPort:          listenPort,
		DNS:                 dnss,
		MTU:                 mtu,
		PersistentKeepalive: persistentKeepalive,
	}

	return apiWGC
}

func (dt *DeviceTemplate) ToAPI() *api.DeviceTemplate {
	var apList []*api.AddressPool
	for _, ap := range dt.AddressPools {
		apList = append(apList, ConvertDBToAPIPool(ap))
	}

	dnss := make([]string, len(dt.DNS))
	copy(dnss, dt.DNS)

	var listenPort, mtu string
	if dt.ListenPort != -1 {
		listenPort = strconv.Itoa(dt.ListenPort)
	}
	if dt.MTU != -1 {
		mtu = strconv.Itoa(dt.MTU)
	}

	apiDT := &api.DeviceTemplate{
		ID:            dt.ID,
		InterfaceName: dt.InterfaceName,
		AddressPools:  apList,
		ListenPort:    listenPort,
		DNS:           dnss,
		MTU:           mtu,
	}
	return apiDT
}

func (wgp *WireGuardAdjacency) ToAPI() *api.WireGuardPeer {
	ips := make([]string, len(wgp.AllowedIPs))
	copy(ips, wgp.AllowedIPs)

	var persistentKeepalive string
	if wgp.PersistentKeepalive != -1 {
		persistentKeepalive = strconv.Itoa(wgp.PersistentKeepalive)
	}

	var endpoint string
	if wgp.Server != nil && wgp.Server.Endpoint != (netip.Addr{}) && wgp.ListenPort != -1 {
		endpoint = fmt.Sprintf("%s:%d", wgp.Server.Endpoint.String(), wgp.ListenPort)
	}

	apiPeer := &api.WireGuardPeer{
		ID: wgp.ID,
		Config: &api.WireGuardPeerConfig{
			PublicKey:           wgp.PublicKey,
			PresharedKey:        wgp.PresharedKey,
			AllowedIPs:          ips,
			OtherSideAllowedIPs: wgp.OtherSideAllowedIPs,
			Endpoint:            endpoint,
			PersistentKeepalive: persistentKeepalive,
		},
	}

	if wgp.Server != nil {
		apiServer := wgp.Server.ToAPI()
		apiPeer.Server = &apiServer
	} else {
		apiPeer.DeviceID = wgp.DeviceID
	}

	return apiPeer
}

func validateIPCidr(s string) error {
	var addrStr, cidrStr string

	i := strings.IndexByte(s, '/')
	if i < 0 {
		addrStr = s
	} else {
		addrStr, cidrStr = s[:i], s[i+1:]
	}

	addr, err := netip.ParseAddr(addrStr)
	if err != nil {
		return fmt.Errorf("could not parse IP address: %w", err)
	}

	if len(cidrStr) > 0 {
		cidr, err := strconv.Atoi(cidrStr)
		if err != nil || cidr < 0 || cidr > 128 {
			return fmt.Errorf("invalid network prefix length: %s", s)
		}
		if cidr > 32 && addr.Is4() {
			return fmt.Errorf("invalid network prefix length: %s", s)
		}
	}
	return nil
}

func ValidateKeyBase64(s string) error {
	k, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("could not decode from base64: %v", err)
	}
	if len(k) != keyLength {
		return fmt.Errorf("keys must decode to exactly %v bytes", keyLength)
	}
	return nil
}

func validateInterfaceName(iface string) error {
	if len(iface) > maxInterfaceNameLength {
		return fmt.Errorf("name exceeds %v characters", maxInterfaceNameLength)
	}

	// check if interface name contains whitespace characters or forward slashes
	if strings.ContainsAny(iface, " \f\n\r\t\v//") {
		return errors.New("name cannot contain whitespace characters or forward slashes")
	}
	return nil
}

// ParseIPNet not only validates address, but also translates:
//
//	"10.10.10.5/24"     -> "10.10.10.0/24"
//	"10.10.10.5"        -> "10.10.10.5/32"
//	"2001:db8:5a3::/32" -> "2001:db8::/32"
//	"2001:db8::2021"    -> "2001:db8::2021/128"
func ParseIPNet(address string) (netip.Prefix, error) {
	if address == "" {
		return netip.Prefix{}, fmt.Errorf("address cannot be an empty string")
	}

	if strings.Contains(address, "/") {
		prefix, err := netip.ParsePrefix(address)
		if err != nil {
			return netip.Prefix{}, err
		}
		return prefix.Masked(), nil
	} else {
		addr, err := netip.ParseAddr(address)
		if err != nil {
			return netip.Prefix{}, err
		}
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
}
