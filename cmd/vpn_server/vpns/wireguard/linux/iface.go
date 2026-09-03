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
	"os"

	"github.com/vishvananda/netlink"
)

var (
	ErrUseAfterDestroy = errors.New("interface cannot be used after Destroy() method call")
)

type Interface struct {
	link *netlink.Wireguard
}

func NewWireGuardInterface(name string) (*Interface, error) {
	if name == "" {
		return nil, errors.New("interface name cannot be an empty string")
	}
	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = name
	wgLink := &netlink.Wireguard{
		LinkAttrs: linkAttrs,
	}
	err := netlink.LinkAdd(wgLink)
	if err != nil {
		if os.IsExist(err) {
			return nil, errors.New("interface with that name already exists")
		}
		return nil, fmt.Errorf("LinkAdd error: %w", err)
	}

	return &Interface{wgLink}, nil
}

func (wgi *Interface) AddAddress(addr string) error {
	if wgi.link == nil {
		return ErrUseAfterDestroy
	}
	if addr == "" {
		return errors.New("address cannot be an empty string")
	}
	a, err := netlink.ParseAddr(addr)
	if err != nil {
		return fmt.Errorf("invalid address: %v", err)
	}
	if err := netlink.AddrAdd(wgi.link, a); err != nil {
		return fmt.Errorf("AddrAdd error: %w", err)
	}
	return nil
}

func (wgi *Interface) RemoveAddress(addr string) error {
	if wgi.link == nil {
		return ErrUseAfterDestroy
	}
	if addr == "" {
		return errors.New("address cannot be an empty string")
	}
	a, err := netlink.ParseAddr(addr)
	if err != nil {
		return fmt.Errorf("invalid address: %v", err)
	}
	if err := netlink.AddrDel(wgi.link, a); err != nil {
		return fmt.Errorf("AddrDel error: %w", err)
	}
	return nil
}

func (wgi *Interface) AddRoute(dest string) error {
	if wgi.link == nil {
		return ErrUseAfterDestroy
	}
	if dest == "" {
		return errors.New("destination cannot be an empty string")
	}
	_, d, err := net.ParseCIDR(dest)
	if err != nil {
		return fmt.Errorf("invalid destination: %v", err)
	}

	route := &netlink.Route{
		LinkIndex: wgi.link.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       d,
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("RouteAdd error: %w", err)
	}
	return nil
}

func (wgi *Interface) RemoveRoute(dest string) error {
	if wgi.link == nil {
		return ErrUseAfterDestroy
	}
	if dest == "" {
		return errors.New("destination cannot be an empty string")
	}
	_, d, err := net.ParseCIDR(dest)
	if err != nil {
		return fmt.Errorf("invalid destination: %v", err)
	}

	route := &netlink.Route{
		LinkIndex: wgi.link.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       d,
	}
	if err := netlink.RouteDel(route); err != nil {
		return fmt.Errorf("RouteDel error: %w", err)
	}
	return nil
}

func (wgi *Interface) SetMTU(mtu int) error {
	if wgi.link == nil {
		return ErrUseAfterDestroy
	}
	if mtu <= 0 {
		return errors.New("MTU value cannot be less than or equal to zero")
	}
	if err := netlink.LinkSetMTU(wgi.link, mtu); err != nil {
		return fmt.Errorf("LinkSetMTU error: %w", err)
	}
	return nil
}

func (wgi *Interface) Up() error {
	if wgi.link == nil {
		return ErrUseAfterDestroy
	}
	err := netlink.LinkSetUp(wgi.link)
	if err != nil {
		return fmt.Errorf("LinkSetUp error: %w", err)
	}
	return nil
}

func (wgi *Interface) Rename(newName string) error {
	err := netlink.LinkSetName(wgi.link, newName)
	if err != nil {
		return fmt.Errorf("can't set the link name: %w", err)
	}
	return nil
}

func (wgi *Interface) Destroy() error {
	err := netlink.LinkDel(wgi.link)
	wgi.link = nil
	if err != nil {
		return fmt.Errorf("could not remove a WireGuard interface: %v", err)
	}
	return nil
}
