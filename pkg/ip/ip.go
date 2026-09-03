/*
 * Copyright 2023 PANTHEON.tech s.r.o.
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

// Package ip complements package netip by providing additional functions for
// IP addresses. It also defines a type AddrRange.
//
// Whenever this package mentions "prefix", it actually means full IP address in
// CIDR notation, stored as (unmasked) netip.Prefix type.
//
// This package supports only IPv4 addresses.
package ip

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// ToSubnetAddr returns address of the subnet that contains prefix.
func ToSubnetAddr(prefix netip.Prefix) netip.Prefix {
	return prefix.Masked()
}

// ToBroadcastAddress returns broadcast address of the subnet that contains prefix.
func ToBroadcastAddress(prefix netip.Prefix) (netip.Prefix, error) {
	addr4 := prefix.Addr().As4()
	pref, err := netip.ParsePrefix("255.255.255.255/" + fmt.Sprint(prefix.Bits()))
	if err != nil {
		return netip.Prefix{}, err
	}

	mask4 := pref.Masked().Addr().As4()
	for i := 0; i < 4; i++ {
		addr4[i] = addr4[i] | ^mask4[i]
	}
	return netip.PrefixFrom(netip.AddrFrom4(addr4), prefix.Bits()), nil
}

// IsSubnetAddr reports whether prefix is a subnet address.
func IsSubnetAddr(prefix netip.Prefix) bool {
	return prefix == ToSubnetAddr(prefix)
}

// IsBroadcastAddress reports whether prefix is a broadcast address.
func IsBroadcastAddress(prefix netip.Prefix) (bool, error) {
	broadcastAddress, err := ToBroadcastAddress(prefix)
	if err != nil {
		return false, fmt.Errorf("ToBroadcastAddress: %v", err)
	}
	return prefix == broadcastAddress, nil
}

// AddrRange defines an address range: a set of consecutive addresses in CIDR
// notation. All addresses in address range are in the same subnet, but they do
// not need to span the whole subnet.
//
// AddrRange is netmask-sensitive. For example, the address range
// 1.2.3.50-1.2.3.99/24 contains addresses 1.2.3.50/24, 1.2.3.51/24, ...,
// 1.2.3.99/24, but not 1.2.3.50/25.
//
// AddrRange is immutable.
type AddrRange struct {
	lower netip.Addr
	upper netip.Addr
	bits  int
}

// LoAddr returns the lower bound of the range as netip.Addr.
func (ran AddrRange) LoAddr() netip.Addr {
	return ran.lower
}

// LoPrefix returns the lower bound of the range as netip.Prefix.
func (ran AddrRange) LoPrefix() netip.Prefix {
	return netip.PrefixFrom(ran.lower, ran.bits)
}

// UpAddr returns the upper bound of the range as netip.Addr.
func (ran AddrRange) UpAddr() netip.Addr {
	return ran.upper
}

// UpPrefix returns the upper bound of the range as netip.Prefix.
func (ran AddrRange) UpPrefix() netip.Prefix {
	return netip.PrefixFrom(ran.upper, ran.bits)
}

// Bits returns netmask lentgh of the addresses.
func (ran AddrRange) Bits() int {
	return ran.bits
}

// AddrRangeFromPrefixes returns address range defined by the lower bound and
// upper bound prefixes (inclusive). The bounds must be in the same subnet.
func AddrRangeFromPrefixes(lower netip.Prefix, upper netip.Prefix) (AddrRange, error) {
	if lower.Bits() != upper.Bits() {
		return AddrRange{}, errors.New("different subnet masks")
	}
	if lower.Addr().Compare(upper.Addr()) == 1 {
		return AddrRange{}, errors.New("wrong order")
	}
	if ToSubnetAddr(lower) != ToSubnetAddr(upper) {
		return AddrRange{}, errors.New("addresses in different subnets")
	}
	return AddrRange{lower.Addr(), upper.Addr(), lower.Bits()}, nil
}

// ParseAddrRange parses string and returns the resulting address range.
//
// Example input:
//
//	"1.2.3.50-1.2.3.99/24"
func ParseAddrRange(s string) (AddrRange, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return AddrRange{}, errors.New("address range does not have two parts")
	}

	lowerA, err := netip.ParseAddr(parts[0])
	if err != nil {
		return AddrRange{}, fmt.Errorf("error parsing lower address: %v", err)
	}
	upperP, err := netip.ParsePrefix(parts[1])
	if err != nil {
		return AddrRange{}, fmt.Errorf("error parsing upper address: %v", err)
	}

	lowerP := netip.PrefixFrom(lowerA, upperP.Bits())

	return AddrRangeFromPrefixes(lowerP, upperP)
}

// MustParseAddrRange calls ParseAddrRange and panics on error.
// It is intended for use in tests with hard-coded address ranges.
func MustParseAddrRange(s string) AddrRange {
	ran, err := ParseAddrRange(s)
	if err != nil {
		panic(err)
	}
	return ran
}

// String returns the address range as a string.
//
// Example:
//
//	"1.2.3.50-1.2.3.99/24"
func (ran AddrRange) String() string {
	return fmt.Sprintf("%s-%s", ran.LoAddr(), ran.UpPrefix())
}

// AddrRangeFromDB parses string in the format as returned from database and
// returns the resulting address range.
//
// Example input:
//
//	"(1.2.3.50/24,1.2.3.99/24)"
func AddrRangeFromDB(s string) (AddrRange, error) {
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return AddrRange{}, errors.New("address range has incorrect format")
	}
	str := strings.TrimPrefix(s, "(")
	str = strings.TrimSuffix(str, ")")

	parts := strings.Split(str, ",")
	if len(parts) != 2 {
		return AddrRange{}, errors.New("address range does not have two parts")
	}

	for i := 0; i < 2; i++ {
		if !strings.Contains(parts[i], "/") {
			parts[i] += "/32"
		}
	}

	lower, err := netip.ParsePrefix(parts[0])
	if err != nil {
		return AddrRange{}, fmt.Errorf("error parsing lower address: %v", err)
	}
	upper, err := netip.ParsePrefix(parts[1])
	if err != nil {
		return AddrRange{}, fmt.Errorf("error parsing upper address: %v", err)
	}

	return AddrRangeFromPrefixes(lower, upper)
}

// ToDB returns the address range as a string suitable to be stored in database.
//
// Example:
//
//	"(1.2.3.50/24,1.2.3.99/24)"
func (ran AddrRange) ToDB() string {
	return fmt.Sprintf("(%s,%s)", ran.LoPrefix(), ran.UpPrefix())
}

// AddrRangeFromSubnet returns address range of all addresses in the subnet.
// If the argument subnet is not a subnet address, AddrRangeFromSubnet returns error.
func AddrRangeFromSubnet(subnet netip.Prefix) (AddrRange, error) {
	if !IsSubnetAddr(subnet) {
		return AddrRange{}, errors.New("not a subnet address")
	}

	broadcastAddress, err := ToBroadcastAddress(subnet)
	if err != nil {
		return AddrRange{}, fmt.Errorf("ToBroadcastAddress: %v", err)
	}

	ran, err := AddrRangeFromPrefixes(subnet, broadcastAddress)
	if err != nil {
		// normally should be unreachable
		return AddrRange{}, fmt.Errorf("AddrRangeFromPrefixes: broadcast address='%s': %v", broadcastAddress.String(), err)
	}
	return ran, nil
}

// MustAddrRangeFromSubnet calls AddrRangeFromSubnet and panics on error.
// It is intended for use in tests with hard-coded subnets.
func MustAddrRangeFromSubnet(subnet netip.Prefix) AddrRange {
	ran, err := AddrRangeFromSubnet(subnet)
	if err != nil {
		panic(err)
	}
	return ran
}

// ToContainingSubnet returns the subnet that contains the address range.
//
// For example, calling ToContainingSubnet on the address range
// 1.2.3.50-1.2.3.99/24 returns the subnet 1.2.3.0/24.
func (ran AddrRange) ToContainingSubnet() (subnet netip.Prefix) {
	return ToSubnetAddr(ran.LoPrefix())
}

// Contains checks whether the address range contains prefix
// (netmask-sensitive).
func (ran AddrRange) Contains(prefix netip.Prefix) bool {
	if prefix.Bits() != ran.Bits() {
		return false
	}
	if prefix.Addr().Compare(ran.LoAddr()) == -1 || prefix.Addr().Compare(ran.UpAddr()) == 1 {
		return false
	}
	return true
}

func addrSetContains(addrSet []netip.Addr, testedAddr netip.Addr) bool {
	for _, addr := range addrSet {
		if testedAddr == addr {
			return true
		}
	}
	return false
}

// FirstAvailableAddress returns the first address from the address range that
// isn't contained in usedAddrs, netmask-insensitive. Error is returned if no
// such address is available.
func (ran AddrRange) FirstAvailableAddress(usedAddrs []netip.Addr) (netip.Prefix, error) {
	bits := ran.Bits()
	chosenAddr := ran.LoAddr()

	if IsSubnetAddr(ran.LoPrefix()) {
		chosenAddr = chosenAddr.Next()
	}

	for {
		if !chosenAddr.IsValid() || chosenAddr.Compare(ran.UpAddr()) == 1 {
			return netip.Prefix{}, fmt.Errorf("no unused address remaining in address range")
		}
		if !addrSetContains(usedAddrs, chosenAddr) {
			break
		}
		chosenAddr = chosenAddr.Next()
	}

	chosenAddrP := netip.PrefixFrom(chosenAddr, bits)

	ok, err := IsBroadcastAddress(chosenAddrP)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("IsBroadcastAddress: broadcast address='%s': %v", chosenAddrP.String(), err)
	}
	if ok {
		return netip.Prefix{}, errors.New("no unused address remaining in address range")
	}
	return chosenAddrP, nil
}
