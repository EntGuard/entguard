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

package vpp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"go.fd.io/govpp/binapi/fib_types"
	interfaces "go.fd.io/govpp/binapi/interface"
	"go.fd.io/govpp/binapi/interface_types"
	"go.fd.io/govpp/binapi/ip"
	"go.fd.io/govpp/binapi/ip_types"
	"go.fd.io/govpp/binapi/vpe"
	"go.fd.io/govpp/binapi/wireguard"

	pbapi "github.com/entguard/entguard/proto/v1"
)

const (
	invalidSwIfIndex = interface_types.InterfaceIndex(^uint32(0))
	invalidPeerIndex = ^uint32(0)

	// useDynamicIndexAllocationForWGInterface is the default and preferred value for
	// `wireguard.WireguardInterface.UserInstance` field. It tells VPP to use dynamic index allocation
	// for WG Interface. For more info see code at
	// https://github.com/FDio/vpp/blob/stable/2406/src/plugins/wireguard/wireguard_if.c#L195
	useDynamicIndexAllocationForWGInterface = 0xFFFFFFFF
)

// addRoute adds route that steers all traffic for given destSubnet into interface given by VPP interface
// index (swIfIndex)
func (c *Client) addRoute(destSubnet string, swIfIndex interface_types.InterfaceIndex) error {
	return c.addRemoveRoute(true, destSubnet, swIfIndex)
}

// removeRoute removes route that steers all traffic for given destSubnet into interface given by VPP interface
// index (swIfIndex)
func (c *Client) removeRoute(destSubnet string, swIfIndex interface_types.InterfaceIndex) error {
	return c.addRemoveRoute(false, destSubnet, swIfIndex)
}

// addRemoveRoute adds/removes route that steers all traffic for given destSubnet into interface given
// by VPP interface index (swIfIndex)
func (c *Client) addRemoveRoute(isAddition bool, destSubnet string, swIfIndex interface_types.InterfaceIndex) error {
	vppClient := ip.NewServiceClient(c.vppConn)

	// Destination subnet conversion
	_, destIPNet, err := net.ParseCIDR(destSubnet)
	if err != nil {
		return fmt.Errorf("failed to parse destination subnet '%s': %v", destSubnet, err)
	}
	destPrefix := c.networkToVPPPrefix(destIPNet)

	req := &ip.IPRouteAddDel{
		// Multi path is always true
		IsMultipath: true,
		IsAdd:       isAddition,
		Route: ip.IPRoute{
			TableID: uint32(0), // assumption is that we are using only VRF 0 in our use cases
			Prefix:  destPrefix,
			NPaths:  1,
			Paths: []fib_types.FibPath{{
				SwIfIndex: uint32(swIfIndex),
				TableID:   0, // assumption is that we are using only VRF 0 in our use cases
			}},
		},
	}

	_, err = vppClient.IPRouteAddDel(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to %s route (dest=%s, destInterface's swIfIndex=%d) due to: %w",
			boolStringSwitch(isAddition, "add", "remove"), destSubnet, swIfIndex, err)
	}

	return nil
}

func (c *Client) setMTU(swIfIndex interface_types.InterfaceIndex, mtu int) error {
	if mtu <= 0 {
		return errors.New("MTU value cannot be less than or equal to zero")
	}
	vppClient := interfaces.NewServiceClient(c.vppConn)

	// Note: there are multiple MTU settings, see https://docs.fd.io/vpp/19.01/md_src_vnet_MTU.html
	// In case of HW interfaces the HW MTU should be set, but we need to setup only Wireguard interface MTU and
	// that interface is virtual, so only SW MTU can be configured
	_, err := vppClient.SwInterfaceSetMtu(context.Background(), &interfaces.SwInterfaceSetMtu{
		SwIfIndex: swIfIndex,
		Mtu: []uint32{
			uint32(mtu), // L3 MTU
			uint32(mtu), // IPv4 MTU
			uint32(mtu), // IPv6 MTU
			uint32(mtu), // MPLS MTU
		},
	})
	if err != nil {
		return fmt.Errorf("failed to set MTU value(%d) due to: %w", mtu, err)
	}
	return nil
}

func (c *Client) addAddress(swIfIndex interface_types.InterfaceIndex, addr string) error {
	return c.addRemoveAddress(swIfIndex, addr, true)
}

func (c *Client) removeAddress(swIfIndex interface_types.InterfaceIndex, addr string) error {
	return c.addRemoveAddress(swIfIndex, addr, false)
}

func (c *Client) addRemoveAddress(swIfIndex interface_types.InterfaceIndex, addr string, isAddition bool) error {
	if addr == "" {
		return errors.New("address cannot be an empty string")
	}
	vppClient := interfaces.NewServiceClient(c.vppConn)

	ipAddr, ipNet, err := net.ParseCIDR(addr)
	if err != nil {
		return fmt.Errorf("parsing address %s failed: %v", addr, err)
	}
	prefixSize, _ := ipNet.Mask.Size()

	_, err = vppClient.SwInterfaceAddDelAddress(context.Background(), &interfaces.SwInterfaceAddDelAddress{
		SwIfIndex: swIfIndex,
		IsAdd:     isAddition,
		Prefix:    ip_types.AddressWithPrefix{Address: c.ipv4ToVPPAddress(ipAddr), Len: byte(prefixSize)},
	})
	if err != nil {
		return fmt.Errorf("failed to %s ip address %s to interface with swIfIndex %d : %v",
			boolStringSwitch(isAddition, "add", "remove"), addr, swIfIndex, err)
	}

	return nil
}

func (c *Client) deletePeer(peerIndex uint32) error {
	vppClient := wireguard.NewServiceClient(c.vppConn)

	_, err := vppClient.WireguardPeerRemove(context.Background(), &wireguard.WireguardPeerRemove{PeerIndex: peerIndex})
	if err != nil {
		return fmt.Errorf("failed to remove peer with index %d due to: %w", peerIndex, err)
	}
	return nil
}

func (c *Client) addPeer(peer *pbapi.WireGuardPeer, wgInterfaceSwIfIndex interface_types.InterfaceIndex) (uint32, error) {
	vppClient := wireguard.NewServiceClient(c.vppConn)

	// required peer settings
	publicKeyBytes, err := base64.StdEncoding.DecodeString(peer.PublicKey)
	if err != nil {
		return invalidPeerIndex, fmt.Errorf("decoding public key failed: %w", err)
	}

	var allowedIps []ip_types.Prefix
	for _, allowedIp := range peer.AllowedIps {
		prefix, err := ip_types.ParsePrefix(allowedIp)
		if err != nil {
			return invalidPeerIndex, fmt.Errorf("parsing allowedIp(%s) failed: %w", allowedIp, err)
		}
		allowedIps = append(allowedIps, prefix)
	}

	// create VPP's peer request struct with required settings
	// Note: peer.PresharedKey functionality is not supported in VPP
	vppPeer := wireguard.WireguardPeer{
		PublicKey:   publicKeyBytes,
		SwIfIndex:   wgInterfaceSwIfIndex,
		TableID:     uint32(0), // assuming that wg-underlay interface is in default VRF(single VRF config solution)
		NAllowedIps: uint8(len(allowedIps)),
		AllowedIps:  allowedIps,
	}

	// apply optional peer settings
	if peer.GetEndpoint() != "" {
		// client endpoint(reachable public ip address) can be unknown ("road warrior" use case:
		// client changes locations ("road warrior") and so does his endpoint, so each time the client tries to
		// connect from somewhere to server, both sides do some handshaking/wg protocol process/... to verify
		// each other and src IP address from client packets will become client endpoint for server)
		endpoint, err := ip_types.ParseAddress(peer.GetEndpoint())
		if err != nil {
			return invalidPeerIndex, fmt.Errorf("parsing endpoint(%s) failed: %w", peer.GetEndpoint(), err)
		}
		vppPeer.Endpoint = endpoint
	}

	if peer.GetPersistentKeepalive() != "" {
		persistentKeepalive, err := strconv.Atoi(peer.PersistentKeepalive)
		if err != nil {
			return invalidPeerIndex, fmt.Errorf("parsing persistentKeepalive(%s) "+
				"failed: %w", peer.PersistentKeepalive, err)
		}
		vppPeer.PersistentKeepalive = uint16(persistentKeepalive)
	}

	// create peer in VPP
	reply, err := vppClient.WireguardPeerAdd(context.Background(), &wireguard.WireguardPeerAdd{Peer: vppPeer})
	if err != nil {
		return 0, fmt.Errorf("failed to add peer to wireguard interface: %v", err)
	}

	return reply.PeerIndex, nil
}

func (c *Client) setInterfaceName(swIfIndex interface_types.InterfaceIndex, name string) error {
	vppClient := interfaces.NewServiceClient(c.vppConn)

	_, err := vppClient.SwInterfaceSetInterfaceName(context.Background(), &interfaces.SwInterfaceSetInterfaceName{
		SwIfIndex: swIfIndex,
		Name:      name,
	})
	return err
}

func (c *Client) setInterfaceStateToUp(swIfIndex interface_types.InterfaceIndex) error {
	vppClient := interfaces.NewServiceClient(c.vppConn)

	_, err := vppClient.SwInterfaceSetFlags(context.Background(), &interfaces.SwInterfaceSetFlags{
		SwIfIndex: swIfIndex,
		// Note: this also clears other non-up-state flags but it is not a problem in our use case
		Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	})
	return err
}

func (c *Client) createWGInterface(privateKey string, port int, sourceIP ip_types.Address) (interface_types.InterfaceIndex, error) {
	vppClient := wireguard.NewServiceClient(c.vppConn)

	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return invalidSwIfIndex, fmt.Errorf("failed to decode private key(%s) "+
			"from base64 string to []byte due to: %v", privateKey, err)
	}
	requestedWGInterface := wireguard.WireguardInterface{
		UserInstance: useDynamicIndexAllocationForWGInterface,
		PrivateKey:   privateKeyBytes,
		Port:         uint16(port),
		SrcIP:        sourceIP,
	}
	reply, err := vppClient.WireguardInterfaceCreate(context.Background(), &wireguard.WireguardInterfaceCreate{
		Interface:   requestedWGInterface,
		GenerateKey: false,
	})
	if err != nil {
		return invalidSwIfIndex, fmt.Errorf("failed to create wg interface (with port %d) in VPP due to: %w",
			requestedWGInterface.Port, err)
	}

	return reply.SwIfIndex, nil
}

func (c *Client) deleteWGInterface(wgSwIfIndex interface_types.InterfaceIndex) error {
	if wgSwIfIndex == invalidSwIfIndex {
		return fmt.Errorf("got invalid interface index(swIfIndex)")
	}

	vppClient := wireguard.NewServiceClient(c.vppConn)
	_, err := vppClient.WireguardInterfaceDelete(context.Background(), &wireguard.WireguardInterfaceDelete{
		SwIfIndex: wgSwIfIndex,
	})
	if err != nil {
		return fmt.Errorf("failed to delete WG interface with swIfIndex %d: %w", wgSwIfIndex, err)
	}
	return nil
}

func (c *Client) retrieveVPPInfo() (*vpe.ShowVersionReply, error) {
	vppClient := vpe.NewServiceClient(c.vppConn)

	reply, err := vppClient.ShowVersion(context.Background(), &vpe.ShowVersion{})
	if err != nil {
		return nil, fmt.Errorf("getting VPP version info failed: %w", err)
	}
	return reply, nil
}

func (c *Client) dumpInterfaces() ([]*interfaces.SwInterfaceDetails, error) {
	vppClient := interfaces.NewServiceClient(c.vppConn)

	stream, err := vppClient.SwInterfaceDump(context.Background(), &interfaces.SwInterfaceDump{
		SwIfIndex: ^interface_types.InterfaceIndex(0),
	})
	if err != nil {
		return nil, fmt.Errorf("listing interfaces failed: %w", err)
	}
	interfaces := make([]*interfaces.SwInterfaceDetails, 0)
	for {
		iface, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receiving interface list failed: %w", err)
		}
		interfaces = append(interfaces, iface)
	}
	return interfaces, nil
}

func (c *Client) retrieveInterfaceIPAddresses(swIfIndex interface_types.InterfaceIndex) ([]*ip.IPAddressDetails, error) {
	vppClient := ip.NewServiceClient(c.vppConn)

	stream, err := vppClient.IPAddressDump(context.Background(), &ip.IPAddressDump{
		SwIfIndex: swIfIndex,
		IsIPv6:    false,
	})
	if err != nil {
		return nil, fmt.Errorf("listing ip addresses for interface "+
			"with swIfIndex %v failed: %w", swIfIndex, err)
	}
	addressDetails := make([]*ip.IPAddressDetails, 0)
	for {
		iface, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receiving ip address list for interface with swIfIndex %v "+
				"failed: %w", swIfIndex, err)
		}
		addressDetails = append(addressDetails, iface)
	}
	return addressDetails, nil
}

func (c *Client) ipv4ToVPPAddress(address net.IP) (ipAddr ip_types.Address) {
	ipAddr.Af = ip_types.ADDRESS_IP4
	var ip4addr ip_types.IP4Address
	copy(ip4addr[:], address.To4())
	ipAddr.Un.SetIP4(ip4addr)
	return
}

func (c *Client) networkToVPPPrefix(dstNetwork *net.IPNet) ip_types.Prefix {
	mask, _ := dstNetwork.Mask.Size()
	return ip_types.Prefix{
		Address: c.ipv4ToVPPAddress(dstNetwork.IP),
		Len:     uint8(mask),
	}
}

func boolStringSwitch(condition bool, trueOutput, falseOutput string) string {
	if condition {
		return trueOutput
	}
	return falseOutput
}

func structSliceToPrintableString[T any](slice []T) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, item := range slice {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%+v", item) // format to have struct fields in output
	}
	sb.WriteString("]")
	return sb.String()
}
