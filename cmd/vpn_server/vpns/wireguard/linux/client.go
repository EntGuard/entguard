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

package linux

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl"

	"github.com/entguard/entguard/pkg/validation"
	pbapi "github.com/entguard/entguard/proto/v1"
)

var (
	// ErrAlreadyRunning is returned when there is at least one WireGuard interface.
	ErrAlreadyRunning = errors.New("WireGuard is already running")
	// ErrAlreadyManaging is returned when user wants to add one more interface.
	ErrAlreadyManaging = errors.New("the client already manages the WireGuard interface")
	// ErrNotManaging is returned when there is no interface to update or remove.
	ErrNotManaging = errors.New("the client does not manage any WireGuard interface")
)

// Client handles wireguard tunnel on server side. That includes not only wireguard-specific configuration
// like peers, but also some hidden close related configuration (i.e. wg interface creation using netlink in linux
// client implementation) that can't be statically created at deployment time (day0 configuration applied by egvpn tool)
type Client struct {
	log     *log.Entry
	wgIface *Interface
	wgCtrl  *wgctrl.Client
	wgCfg   *pbapi.WireGuard
	wgDev   *DeviceConfig
}

// NewClient returns a client to manage a WireGuard interface.
func NewClient(l *log.Entry) (*Client, error) {
	wgCtrl, err := wgctrl.New()
	if err != nil {
		return nil, err
	}

	logger := l.WithField("reportCaller", "wg-linux")
	devs, err := wgCtrl.Devices()
	if err != nil {
		return nil, err
	}

	if len(devs) != 0 {
		for _, d := range devs {
			logger.WithFields(log.Fields{
				"name":          d.Name,
				"numberOfPeers": len(d.Peers),
			}).Warn("Found WireGuard interface")
		}
		return nil, ErrAlreadyRunning
	}

	c := &Client{
		log:    logger,
		wgCtrl: wgCtrl,
	}
	return c, nil
}

// UpdateWireGuardInterface updates existing WireGuard interface (it does not update peers)
func (c *Client) UpdateWireGuardInterface(newWGC *pbapi.WireGuard) error {
	if c.wgIface == nil {
		return ErrNotManaging
	}
	if newWGC == nil {
		return fmt.Errorf("new wg config can't be nil")
	}
	if newWGC.Iface == nil {
		return fmt.Errorf("new wg config's interface can't be nil")
	}

	if err := validation.ValidateWGConfig(newWGC); err != nil {
		return fmt.Errorf("invalid new configuration: %w", err)
	}

	if c.wgCfg.Name != newWGC.Name { // rename interface
		c.log.WithField("newName", newWGC.Name).
			Debug("renaming wg interface")
		err := c.wgIface.Rename(newWGC.Name)
		if err != nil {
			return fmt.Errorf("could not rename WireGuard interface to %v: %w", newWGC.Name, err)
		}
	}

	if c.wgCfg.Iface.Mtu != newWGC.Iface.Mtu {
		c.log.WithField("newMTU", newWGC.Iface.Mtu).
			Debug("changing MTU for wg interface")
		mtu, err := strconv.Atoi(newWGC.Iface.Mtu)
		if err != nil {
			return fmt.Errorf("bad MTU value %q: %w", newWGC.Iface.Mtu, err)
		}
		err = c.wgIface.SetMTU(mtu)
		if err != nil {
			return fmt.Errorf("could not set new MTU(%d) to WireGuard interface: %w", mtu, err)
		}
	}

	toAdd, toRemove := diffAddresses(c.wgCfg.Iface.Addresses, newWGC.Iface.Addresses)
	for _, addr := range toAdd {
		c.log.WithField("address", addr).
			Debug("changing wg interface ip addreesses: adding ip address")
		err := c.wgIface.AddAddress(addr)
		if err != nil {
			return fmt.Errorf("could not add address(%v) for WireGuard interface: %w", addr, err)
		}
	}
	for _, addr := range toRemove {
		c.log.WithField("address", addr).
			Debug("changing wg interface ip addreesses: removing ip address")
		err := c.wgIface.RemoveAddress(addr)
		if err != nil {
			return fmt.Errorf("could not remove address(%v) for WireGuard interface: %w", addr, err)
		}
	}

	var newDeviceConfig *DeviceConfig
	if c.wgCfg.Iface.PrivateKey != newWGC.Iface.PrivateKey || c.wgCfg.Iface.ListenPort != newWGC.Iface.ListenPort {
		c.log.WithFields(log.Fields{
			"newPrivateKey": newWGC.Iface.PrivateKey,
			"newListenPort": newWGC.Iface.ListenPort,
		}).Debug("Scheduling changing WireGuard configuration (interface private key and listen port)")
		var err error
		newDeviceConfig, err = NewDeviceConfig(newWGC.Iface.PrivateKey, newWGC.Iface.ListenPort)
		if err != nil {
			return fmt.Errorf("invalid private key(%v) or listen port(%v) configuration: %w",
				newWGC.Iface.PrivateKey, newWGC.Iface.ListenPort, err)
		}
		config, err := newDeviceConfig.Done()
		if err != nil {
			return fmt.Errorf("failing to build device configuration (private key(%v),listen port(%v)): %w",
				newWGC.Iface.PrivateKey, newWGC.Iface.ListenPort, err)
		}
		c.log.Debug("Applying scheduled changes (private key, listen port)")
		err = c.wgCtrl.ConfigureDevice(newWGC.Name, *config)
		if err != nil {
			return fmt.Errorf("could not change private key(%v) or listen port(%v) for WireGuard "+
				"interface: %w", newWGC.Iface.PrivateKey, newWGC.Iface.ListenPort, err)
		}
	}

	// update remembered configuration
	// (Note: Normally this update should happen after successful update of everything (that includes things updated
	// outside of this function) as some kind of successful transaction commit (there is transaction though!).
	// The reason why it is here is that if something fails in update after this point (outside of this function)
	// then reconfiguration from scratch happens that will also delete this client and recreate it so no harm
	// to set it here instead of making it from outside of client with some new client function that just messes
	// with client internal configuration)
	if newDeviceConfig != nil {
		c.wgDev = newDeviceConfig // update the remembered device config used later for peer updates (after successful update)
	}
	c.wgCfg = newWGC

	return nil
}

// AddWireGuard creates local WireGuard tunnel end (properly configured Wireguard link, Wireguard peers, routes,....).
func (c *Client) AddWireGuard(wgc *pbapi.WireGuard) error {
	if c.wgIface != nil {
		return ErrAlreadyManaging
	}

	if err := validation.ValidateWGConfig(wgc); err != nil {
		return fmt.Errorf("invalid configuration: %v", err)
	}

	c.log.WithFields(log.Fields{
		"privateKey": wgc.Iface.PrivateKey,
		"listenPort": wgc.Iface.ListenPort,
	}).Debug("Scheduling creating WireGuard configuration (interface private key and listen port)")
	devConfig, err := NewDeviceConfig(wgc.Iface.PrivateKey, wgc.Iface.ListenPort)
	if err != nil {
		return fmt.Errorf("invalid configuration: %v", err)
	}
	c.log.Debug("Preparing to schedule creating peers")
	devConfig.Create()
	for _, p := range wgc.GetPeers() {
		c.log.WithFields(log.Fields{
			"publicKey":  p.PublicKey,
			"allowedIPs": p.AllowedIps,
		}).Debug("Scheduling to add peer")
		if err := devConfig.AddPeerE(p); err != nil {
			return err
		}
	}
	cfg, _ := devConfig.Done()
	c.wgDev = devConfig

	c.wgCfg = wgc

	c.log.WithField("interfaceName", wgc.Name).
		Debug("Creating a new interface")
	c.wgIface, err = NewWireGuardInterface(wgc.Name)
	if err != nil {
		return fmt.Errorf("could not create a WireGuard interface with name %q: %v", wgc.Name, err)
	}

	for _, addr := range wgc.Iface.Addresses {
		c.log.WithFields(log.Fields{
			"interface": wgc.Name,
			"address":   addr,
		}).Debug("Assigning address to interface")
		err := c.wgIface.AddAddress(addr)
		if err != nil {
			c.deleteInterface()
			return fmt.Errorf("could not assign the address %q to the interface %s: %v", addr, wgc.Name, err)
		}
	}
	if wgc.Iface.Mtu != "" {
		mtu, err := strconv.Atoi(wgc.Iface.Mtu)
		if err != nil {
			c.deleteInterface()
			return fmt.Errorf("bad MTU value %q: %v", wgc.Iface.Mtu, err)
		}
		c.log.WithFields(log.Fields{
			"MTU":       wgc.Iface.Mtu,
			"interface": wgc.Name,
		}).Debug("Setting MTU")
		err = c.wgIface.SetMTU(mtu)
		if err != nil {
			c.deleteInterface()
			return fmt.Errorf("could not set MTU to the interface %q: %v", wgc.Iface.Mtu, err)
		}
	}

	c.log.Debug("Applying scheduled changes (private key, listen port, adding peers)")
	err = c.wgCtrl.ConfigureDevice(wgc.Name, *cfg)
	if err != nil {
		c.deleteInterface()
		if os.IsNotExist(err) {
			// This will be very weird, because the interface was successfully created few lines above.
			return fmt.Errorf("interface %q does not exist", wgc.Name)
		}
		return err
	}

	c.log.WithField("interface", wgc.Name).
		Debug("Bringing interface up")
	err = c.wgIface.Up()
	if err != nil {
		c.deleteInterface()
		return err
	}

	for _, peer := range wgc.Peers {
		for _, dest := range peer.AllowedIps {
			c.log.WithFields(log.Fields{
				"interface":   wgc.Name,
				"destination": dest,
			}).Debug("Adding route via interface")
			err := c.wgIface.AddRoute(dest)
			if err != nil {
				c.deleteInterface()
				return fmt.Errorf("could not add route %q over interface %s: %v", dest, wgc.Name, err)
			}
		}
	}

	return nil
}

// AddPeers adds the list of peers to the WireGuard interface.
// It also adds steering routes for allowed IPs of given peers.
func (c *Client) AddPeers(peers ...*pbapi.WireGuardPeer) error {
	if c.wgIface == nil {
		// or not added? :thinking:
		return ErrNotManaging
	}

	// add peers to wg interface
	c.log.Debug("Preparing to schedule updating peers (adding peers)")
	dev := c.wgDev.Update()
	for _, p := range peers {
		c.log.WithFields(log.Fields{
			"publicKey":  p.PublicKey,
			"allowedIPs": p.AllowedIps,
		}).Debug("Scheduling to add peer")
		dev.AddPeer(p)
	}
	cfg, err := dev.Done()
	if err != nil {
		return err
	}
	c.log.Debug("Applying scheduled changes (adding peers)")
	err = c.wgCtrl.ConfigureDevice(c.wgCfg.Name, *cfg)
	if err != nil {
		return err
	}

	// add routes to steer traffic into peer's allowed IPs
	for _, peer := range peers {
		for _, dest := range peer.AllowedIps {
			c.log.WithFields(log.Fields{
				"interface":   c.wgCfg.Name,
				"destination": dest,
			}).Debug("Adding route via interface")
			err = c.wgIface.AddRoute(dest)
			if err != nil {
				return fmt.Errorf("could not add route %q over interface %s: %v", dest, c.wgCfg.Name, err)
			}
		}
	}

	return nil
}

// DelPeers deletes the list of peers from the WireGuard interface.
// It also removes steering routes for allowed IPs of given peers.
func (c *Client) DelPeers(peers ...*pbapi.WireGuardPeer) error {
	if c.wgIface == nil {
		// or not added? :thinking:
		return ErrNotManaging
	}

	// remove some peers from wg interface
	c.log.Debug("Preparing to schedule updating peers (deleting peers)")
	dev := c.wgDev.Update()
	for _, p := range peers {
		c.log.WithFields(log.Fields{
			"publicKey": p.PublicKey,
		}).Debug("Scheduling to delete peer")
		dev.DelPeer(p)
	}
	cfg, err := dev.Done()
	if err != nil {
		return err
	}
	c.log.Debug("Applying scheduled changes (deleting peers)")
	err = c.wgCtrl.ConfigureDevice(c.wgCfg.Name, *cfg)
	if err != nil {
		return err
	}

	// remove corresponding routes that are steering traffic into peer's allowed IPs
	for _, peer := range peers {
		for _, dest := range peer.AllowedIps {
			c.log.WithFields(log.Fields{
				"interface":   c.wgCfg.Name,
				"destination": dest,
			}).Debug("Removing route via interface")
			err = c.wgIface.RemoveRoute(dest)
			if err != nil {
				return fmt.Errorf("could not remove route %q over interface %s: %v", dest, c.wgCfg.Name, err)
			}
		}
	}

	return nil
}

// Close does cleanup and closes the client. Do not use the client after close.
func (c *Client) Close() {
	err := c.wgCtrl.Close()
	if err != nil {
		c.log.Error(err)
	}
	c.deleteInterface() // this will clear also routes steering traffic into wg interface (peer.AllowedIps routes)
}

func (c *Client) deleteInterface() {
	if c.wgIface != nil {
		c.log.WithField("interface", c.wgCfg.Name).
			Debug("Deleting interface")
		err := c.wgIface.Destroy()
		if err != nil {
			c.log.Error(err)
		}
		c.wgIface = nil
	}
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
