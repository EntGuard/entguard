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
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"go.fd.io/govpp"
	"go.fd.io/govpp/adapter/socketclient"
	interfaces "go.fd.io/govpp/binapi/interface"
	"go.fd.io/govpp/binapi/interface_types"
	"go.fd.io/govpp/core"

	"github.com/entguard/entguard/pkg/validation"
	pbapi "github.com/entguard/entguard/proto/v1"
)

const (
	wgUnderlayInterfaceTag = "wg-underlay"
	connectionRetryPeriod  = 60 * time.Second
)

var (
	// ErrAlreadyManaging is returned when user wants to add one more interface.
	ErrAlreadyManaging = errors.New("the client already manages the WireGuard interface")
	// ErrNotManaging is returned when there is no interface to update or remove.
	ErrNotManaging = errors.New("the client does not manage any WireGuard interface")
)

// Client handles wireguard tunnel on server side. That includes not only wireguard-specific configuration
// like peers, but also some hidden close related configuration (i.e. wg interface creation using netlink in linux
// client implementation) that can't be statically created at deployment time (day0 configuration applied by egvpn tool)
type Client struct {
	log                *log.Entry
	wgCfg              *pbapi.WireGuard
	vppConn            *core.Connection
	exitConnLoggerLoop func()

	// tracking of VPP state
	wgSwIfIndex         interface_types.InterfaceIndex
	peerToIndexMapping  map[*pbapi.WireGuardPeer]uint32
	destOfCreatedroutes map[string]struct{} // same string format as peer.AllowedIPs
}

// NewClient returns a client to manage a WireGuard interface.
func NewClient(l *log.Entry) (*Client, error) {
	logger := l.WithField("reportCaller", "wg-VPP")

	// Note: AsyncConnect is retry-able connect and also later in case of disconnection it tries to reconnect
	// (with new retry-able connect) so only way how to invalidate returned connection instance is when VPP
	// is down (or otherwise inaccessible) for longer then connectionRetryPeriod -> using retrieved connection
	// without any kind of manual renew or reconnect for the whole lifetime of client
	logger.Debug("starting async connect to VPP")
	conn, connEv, err := govpp.AsyncConnect(socketclient.DefaultSocketName,
		int(connectionRetryPeriod/core.DefaultReconnectInterval), core.DefaultReconnectInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to start async connecting to VPP: %v", err)
	}

	logger.Debug("waiting for govpp to connect to VPP")
	e := <-connEv
	if e.State != core.Connected {
		return nil, fmt.Errorf("failed to connect to VPP (received connection event %+v)", e)
	}

	logger.Debug("starting listening to connection events")
	exitConnLoggerLoop := runConnectionLoggerLoop(connEv, logger)

	// init client struct
	c := &Client{
		log:                 logger,
		vppConn:             conn,
		exitConnLoggerLoop:  exitConnLoggerLoop,
		wgSwIfIndex:         invalidSwIfIndex,
		peerToIndexMapping:  make(map[*pbapi.WireGuardPeer]uint32),
		destOfCreatedroutes: map[string]struct{}{},
	}

	// simple API usage check
	info, err := c.retrieveVPPInfo()
	if err != nil {
		return nil, fmt.Errorf("simple API check(retrieving VPP info) failed: %v", err)
	}
	c.log.WithField("vppVersion", info.Version).Info("successfully connected to VPP")

	return c, nil
}

// UpdateWireGuardInterface updates existing WireGuard interface (it does not update peers)
func (c *Client) UpdateWireGuardInterface(newWGC *pbapi.WireGuard) error {
	if c.wgSwIfIndex == invalidSwIfIndex {
		return ErrNotManaging
	}

	if err := validation.ValidateWGConfig(newWGC); err != nil {
		return fmt.Errorf("invalid new configuration: %w", err)
	}

	// handling changes that VPP's API is not able to update (-> recreation)
	recreate := false
	if c.wgCfg.Iface.PrivateKey != newWGC.Iface.PrivateKey {
		c.log.WithField("privateKey", newWGC.Iface.PrivateKey).Debug("wg interface's privateKey " +
			"change detected. Update of privateKey is unsupported via VPP API. Therefore recreation of WG " +
			"interface/peers/routes in VPP is initialized")
		recreate = true
	}
	if c.wgCfg.Iface.ListenPort != newWGC.Iface.ListenPort {
		c.log.WithField("listenPort", newWGC.Iface.ListenPort).Debug("wg interface's listenPort " +
			"change detected. Update of listenPort is unsupported via VPP API. Therefore recreation of WG " +
			"interface/peers/route in VPP is initialized")
		recreate = true
	}
	if recreate {
		c.log.Debug("cleaning previous VPP configuration...")
		if err := c.cleanVPP(); err != nil {
			return fmt.Errorf("couldn't properly clean VPP configuration for vpp config recreation: %w", err)
		}
		c.cleanVPPObjectTracking()

		c.log.Debug("applying new VPP configuration...")
		if err := c.AddWireGuard(newWGC); err != nil {
			return fmt.Errorf("could not add wireguard to VPP(reconfiguration "+
				"due to unsupported update of wg privateKey/port): %w", err)
		}
		return nil // recreation already reconfigured things according to new config -> we are done here
	}

	// handling VPP API supported updates
	if c.wgCfg.Name != newWGC.Name { // rename interface
		c.log.WithField("newInterfaceName", newWGC.Name).Debug("renaming wg interface")
		err := c.setInterfaceName(c.wgSwIfIndex, newWGC.Name)
		if err != nil {
			return fmt.Errorf("could not rename WireGuard interface to %v: %w", newWGC.Name, err)
		}
	}

	if c.wgCfg.Iface.Mtu != newWGC.Iface.Mtu {
		c.log.WithField("newMTU", newWGC.Iface.Mtu).Debug("changing MTU for wg interface")
		mtu, err := strconv.Atoi(newWGC.Iface.Mtu)
		if err != nil {
			return fmt.Errorf("bad MTU value %q: %w", newWGC.Iface.Mtu, err)
		}
		err = c.setMTU(c.wgSwIfIndex, mtu)
		if err != nil {
			return fmt.Errorf("could not set new MTU(%d) to WireGuard interface: %w", mtu, err)
		}
	}

	toAdd, toRemove := diffAddresses(c.wgCfg.Iface.Addresses, newWGC.Iface.Addresses)
	for _, addr := range toAdd {
		c.log.WithField("address", addr).Debug("changing wg interface ip addresses: adding ip address")
		err := c.addAddress(c.wgSwIfIndex, addr)
		if err != nil {
			return fmt.Errorf("could not add address(%v) for WireGuard interface: %w", addr, err)
		}
	}
	for _, addr := range toRemove {
		c.log.WithField("address", addr).Debug("changing wg interface ip addresses: removing ip address")
		err := c.removeAddress(c.wgSwIfIndex, addr)
		if err != nil {
			return fmt.Errorf("could not remove address(%v) for WireGuard interface: %w", addr, err)
		}
	}

	// update remembered configuration
	// (Note: Normally this update should happen after successful update of everything (that includes things updated
	// outside of this function) as some kind of successful transaction commit (there is transaction though!).
	// The reason why it is here is that if something fails in update after this point (outside of this function)
	// then reconfiguration from scratch happens that will also delete this client and recreate it so no harm
	// to set it here instead of making it from outside of client with some new client function that just messes
	// with client internal configuration)
	c.wgCfg = newWGC

	return nil
}

// AddWireGuard creates local WireGuard tunnel end (properly configured Wireguard link, Wireguard peers, routes,....).
func (c *Client) AddWireGuard(wgc *pbapi.WireGuard) error {
	// basic state/input checks
	if c.wgSwIfIndex != invalidSwIfIndex {
		return ErrAlreadyManaging
	}
	if err := validation.ValidateWGConfig(wgc); err != nil {
		return fmt.Errorf("invalid configuration: %v", err)
	}

	// remember for later comparison with new wg config in UpdateWireGuardInterface() func
	c.wgCfg = wgc

	// add WG interface and peers (+also steering routes for allowed IPs for the peers)
	if err := c.addWireguardInterface(wgc); err != nil {
		return fmt.Errorf("could not create wg interface: %w", err)
	}
	if err := c.AddPeers(wgc.GetPeers()...); err != nil {
		return fmt.Errorf("could not add peers to wg interface: %w", err)
	}

	return nil
}

// AddPeers adds the list of peers to the WireGuard interface.
// It also adds steering routes for allowed IPs of given peers.
func (c *Client) AddPeers(peers ...*pbapi.WireGuardPeer) error {
	if c.wgSwIfIndex == invalidSwIfIndex {
		return ErrNotManaging
	}

	for _, peer := range peers {
		// add peer to wg interface
		c.log.WithFields(log.Fields{
			"peer":      peer,
			"swIfIndex": c.wgSwIfIndex,
		}).Debug("Adding peer to interface.")
		peerIndex, err := c.addPeer(peer, c.wgSwIfIndex)
		if err != nil {
			return fmt.Errorf("failed to add peer(%+v) due to: %w", peer, err)
		}
		c.peerToIndexMapping[peer] = peerIndex

		// add routes to steer peer's allowed ips traffic
		for _, dest := range peer.AllowedIps {
			c.log.WithFields(log.Fields{
				"dest":      dest,
				"swIfIndex": c.wgSwIfIndex,
			}).Debug("Adding route over interface.")
			if err := c.addRoute(dest, c.wgSwIfIndex); err != nil {
				return fmt.Errorf("could not add route %s over interface "+
					"with swIfIndex %d: %w", dest, c.wgSwIfIndex, err)
			}
			c.destOfCreatedroutes[dest] = struct{}{}
		}
	}

	return nil
}

// DelPeers deletes the list of peers from the WireGuard interface.
// It also removes steering routes for allowed IPs of given peers.
func (c *Client) DelPeers(peers ...*pbapi.WireGuardPeer) error {
	if c.wgSwIfIndex == invalidSwIfIndex {
		return ErrNotManaging
	}

	for _, peer := range peers {
		c.log.WithFields(log.Fields{
			"peer":      peer,
			"swIfIndex": c.wgSwIfIndex,
		}).Debug("Deleting peer from interface.")
		// find VPP's peer index for given peer
		var foundPeer *pbapi.WireGuardPeer
		foundPeerIndex := invalidPeerIndex
		for p, i := range c.peerToIndexMapping {
			if proto.Equal(p, peer) {
				foundPeer = p
				foundPeerIndex = i
			}
		}
		if foundPeerIndex == invalidPeerIndex {
			return fmt.Errorf("could not find peer index for peer %+v", peer)
		}

		// delete peer by it's index
		err := c.deletePeer(foundPeerIndex)
		if err != nil {
			return fmt.Errorf("failed to remove peer(%+v) "+
				"with index %d due to: %w", peer, foundPeerIndex, err)
		}

		// cleanup the inner peer indexes mapping
		delete(c.peerToIndexMapping, foundPeer)

		// remove routes to steer peer's allowed ips traffic
		for _, dest := range peer.AllowedIps {
			c.log.WithFields(log.Fields{
				"dest":      dest,
				"swIfIndex": c.wgSwIfIndex,
			}).Debug("Removing route over wg interface.")
			if err := c.removeRoute(dest, c.wgSwIfIndex); err != nil {
				return fmt.Errorf("could not remove route %s over interface "+
					"with swIfIndex %d: %w", dest, c.wgSwIfIndex, err)
			}
			delete(c.destOfCreatedroutes, dest)
		}

	}
	return nil
}

// Close does cleanup and closes the client. Do not use the client after close.
func (c *Client) Close() {
	if err := c.cleanVPP(); err != nil {
		c.log.WithError(err).Warn("couldn't properly clean VPP configuration (it could be a possible problem " +
			"for VPP client recreation, i.e. in reconnection process)")
	}
	c.cleanVPPObjectTracking()
	c.exitConnLoggerLoop()
	c.vppConn.Disconnect()
}

// cleanVPP cleans VPP of previously inserted configuration (as tracked internally by client).
func (c *Client) cleanVPP() error {
	if c.wgSwIfIndex == invalidSwIfIndex {
		c.log.Warn("wg interface's VPP index (swIfIndex) is undefined -> skipping VPP config cleanup")
		return nil
	}

	// delete routes for peer's allowed IPs
	for dest := range c.destOfCreatedroutes {
		if err := c.removeRoute(dest, c.wgSwIfIndex); err != nil {
			return fmt.Errorf("can't cleanup route(dest=%s) in vpp: %w", dest, err)
		}
	}

	// delete WG interface (-> it should take down all interface settings and also WG peers)
	if err := c.deleteWGInterface(c.wgSwIfIndex); err != nil {
		return fmt.Errorf("can't cleanup wg interface in VPP: %w", err)
	}

	return nil
}

func (c *Client) cleanVPPObjectTracking() {
	c.wgSwIfIndex = invalidSwIfIndex
	c.peerToIndexMapping = map[*pbapi.WireGuardPeer]uint32{}
	c.destOfCreatedroutes = map[string]struct{}{}
}

func runConnectionLoggerLoop(connEv chan core.ConnectionEvent, logger *log.Entry) func() {
	stop := make(chan struct{})
	exited := make(chan struct{})
	gracefulShutdown := func() {
		close(stop)
		<-exited
	}
	go connectionLoggerLoop(connEv, logger, stop, exited)
	return gracefulShutdown
}

func connectionLoggerLoop(connEv chan core.ConnectionEvent, logger *log.Entry, stop, exited chan struct{}) {
	for {
		select {
		case <-stop:
			logger.Debug("Exiting connection logger loop.")
			close(exited)
			return
		case event := <-connEv:
			logger.WithField("event", event).Info("VPP connection event detected")
		}
	}
}

// addWireguardInterface adds wireguard interface and properly configures it with the exception of wireguard
// peer configuration.
func (c *Client) addWireguardInterface(wgc *pbapi.WireGuard) error {
	// check the underlay interface that new WG interface will depend on (WG will use it to communicate
	// with external WG peers and it will use it's IP address as source IP for WG encoded packets so that
	// returning WG traffic can be routed correctly back here)
	underlayInterface, err := c.retrieveWGUnderlayInterface()
	if err != nil {
		return fmt.Errorf("could not retrieve underlay interface: %v", err)
	}
	addresses, err := c.retrieveInterfaceIPAddresses(underlayInterface.SwIfIndex)
	if err != nil {
		return fmt.Errorf("retrieving underlay interface IP addresses failed: %v", err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("wireguard underlay interface (interface tagged as %s) doesn't have IP address "+
			"so wireguard interface can't use it properly. Please add IP address to that interface (swIfIndex %v)",
			wgUnderlayInterfaceTag, underlayInterface.SwIfIndex)
	}

	// create WG interface
	listenPort, err := strconv.Atoi(wgc.Iface.ListenPort)
	if err != nil {
		return fmt.Errorf("parsing wg interface port(%s) failed: %w", wgc.Iface.ListenPort, err)
	}
	c.log.WithFields(log.Fields{
		"name":       wgc.Name,
		"listenPort": listenPort,
		"address":    addresses[0].Prefix.Address,
	}).Debug("Creating a new interface.")
	wgSwIndex, err := c.createWGInterface(wgc.Iface.PrivateKey, listenPort, addresses[0].Prefix.Address)
	if err != nil {
		return fmt.Errorf("failed to create wg interface due to: %w", err)
	}
	c.wgSwIfIndex = wgSwIndex

	// configure some basic(non-WG) interface attributes
	if err := c.setInterfaceName(c.wgSwIfIndex, wgc.Name); err != nil {
		return fmt.Errorf("failed to set wg interface name due to: %w", err)
	}
	for _, addr := range wgc.Iface.Addresses {
		c.log.WithFields(log.Fields{
			"address":       addr,
			"interfaceName": wgc.Name,
		}).Debug("Assigning address to interface.")
		if err := c.addAddress(c.wgSwIfIndex, addr); err != nil {
			return fmt.Errorf("could not assign the address %q to the interface %s: %w", addr, wgc.Name, err)
		}
	}
	if wgc.Iface.Mtu != "" {
		mtu, err := strconv.Atoi(wgc.Iface.Mtu)
		if err != nil {
			return fmt.Errorf("bad MTU value %q: %v", wgc.Iface.Mtu, err)
		}
		c.log.WithFields(log.Fields{
			"mtu":           wgc.Iface.Mtu,
			"interfaceName": wgc.Name,
		}).Debug("Setting MTU to interface.")
		if err = c.setMTU(c.wgSwIfIndex, mtu); err != nil {
			return fmt.Errorf("could not set MTU to the interface %q: %w", wgc.Iface.Mtu, err)
		}
	}

	// configured -> set state of wg interface to UP
	c.log.WithField("interface", wgc.Name).Debug("Bringing interface up.")
	if err := c.setInterfaceStateToUp(c.wgSwIfIndex); err != nil {
		return fmt.Errorf("failed to set wg interface state due to: %w", err)
	}
	return nil
}

func (c *Client) retrieveWGUnderlayInterface() (*interfaces.SwInterfaceDetails, error) {
	interfaceList, err := c.dumpInterfaces()
	if err != nil {
		return nil, fmt.Errorf("dumping of the VPP interfaces failed: %v", err)
	}
	var underlayInterface *interfaces.SwInterfaceDetails
	for _, intf := range interfaceList {
		if intf.Tag == wgUnderlayInterfaceTag {
			underlayInterface = intf
			break
		}
	}

	if underlayInterface == nil {
		return nil, fmt.Errorf("no interface with tag %s found (all interfaces: %s)",
			wgUnderlayInterfaceTag, structSliceToPrintableString(interfaceList))
	}
	return underlayInterface, nil
}

func diffAddresses(oldAddrs, newAddrs []string) (toAdd, toRemove []string) {
	if slices.Equal(oldAddrs, newAddrs) {
		return
	}

	var found bool
	// Building a list of addresses which needs to be removed.
	for _, oldAddr := range oldAddrs {
		found = false
		for _, newAddr := range newAddrs {
			if strings.TrimSpace(newAddr) == strings.TrimSpace(oldAddr) {
				found = true
				break
			}
		}
		if !found {
			toRemove = append(toRemove, oldAddr)
		}
	}

	// Building a list of addresses which needs to be added.
	for _, newAddr := range newAddrs {
		found = false
		for _, oldAddr := range oldAddrs {
			if strings.TrimSpace(newAddr) == strings.TrimSpace(oldAddr) {
				found = true
				break
			}
		}
		if !found {
			toAdd = append(toAdd, newAddr)
		}
	}

	return
}
