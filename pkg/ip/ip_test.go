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

package ip

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddressRangeProperties(t *testing.T) {
	// Alternative names if you are doing search for it:
	// TestLoAddr
	// TestLoPrefix
	// TestUpAddr
	// TestUpPrefix
	// TestBits
	// TestString
	// TestToDB

	cases := []struct {
		name         string
		in           AddrRange
		wantLoAddr   netip.Addr
		wantLoPrefix netip.Prefix
		wantUpAddr   netip.Addr
		wantUpPrefix netip.Prefix
		wantBits     int
		wantString   string
		wantToDB     string
	}{
		{
			name:         "standard example",
			in:           MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
			wantLoAddr:   netip.MustParseAddr("1.2.3.50"),
			wantLoPrefix: netip.MustParsePrefix("1.2.3.50/24"),
			wantUpAddr:   netip.MustParseAddr("1.2.3.99"),
			wantUpPrefix: netip.MustParsePrefix("1.2.3.99/24"),
			wantBits:     24,
			wantString:   "1.2.3.50-1.2.3.99/24",
			wantToDB:     "(1.2.3.50/24,1.2.3.99/24)",
		},
		{
			name:         "single address",
			in:           MustParseAddrRange("1.2.3.4-1.2.3.4/24"),
			wantLoAddr:   netip.MustParseAddr("1.2.3.4"),
			wantLoPrefix: netip.MustParsePrefix("1.2.3.4/24"),
			wantUpAddr:   netip.MustParseAddr("1.2.3.4"),
			wantUpPrefix: netip.MustParsePrefix("1.2.3.4/24"),
			wantBits:     24,
			wantString:   "1.2.3.4-1.2.3.4/24",
			wantToDB:     "(1.2.3.4/24,1.2.3.4/24)",
		},
		{
			name:         "23-bit mask",
			in:           MustParseAddrRange("1.2.3.50-1.2.3.99/23"),
			wantLoAddr:   netip.MustParseAddr("1.2.3.50"),
			wantLoPrefix: netip.MustParsePrefix("1.2.3.50/23"),
			wantUpAddr:   netip.MustParseAddr("1.2.3.99"),
			wantUpPrefix: netip.MustParsePrefix("1.2.3.99/23"),
			wantBits:     23,
			wantString:   "1.2.3.50-1.2.3.99/23",
			wantToDB:     "(1.2.3.50/23,1.2.3.99/23)",
		},
		{
			name:         "25-bit mask",
			in:           MustParseAddrRange("1.2.3.50-1.2.3.99/25"),
			wantLoAddr:   netip.MustParseAddr("1.2.3.50"),
			wantLoPrefix: netip.MustParsePrefix("1.2.3.50/25"),
			wantUpAddr:   netip.MustParseAddr("1.2.3.99"),
			wantUpPrefix: netip.MustParsePrefix("1.2.3.99/25"),
			wantBits:     25,
			wantString:   "1.2.3.50-1.2.3.99/25",
			wantToDB:     "(1.2.3.50/25,1.2.3.99/25)",
		},
		{
			name:         "0-bit mask",
			in:           MustParseAddrRange("1.2.3.50-1.2.3.99/0"),
			wantLoAddr:   netip.MustParseAddr("1.2.3.50"),
			wantLoPrefix: netip.MustParsePrefix("1.2.3.50/0"),
			wantUpAddr:   netip.MustParseAddr("1.2.3.99"),
			wantUpPrefix: netip.MustParsePrefix("1.2.3.99/0"),
			wantBits:     0,
			wantString:   "1.2.3.50-1.2.3.99/0",
			wantToDB:     "(1.2.3.50/0,1.2.3.99/0)",
		},
		{
			name:         "32-bit mask",
			in:           MustParseAddrRange("1.2.3.50-1.2.3.50/32"),
			wantLoAddr:   netip.MustParseAddr("1.2.3.50"),
			wantLoPrefix: netip.MustParsePrefix("1.2.3.50/32"),
			wantUpAddr:   netip.MustParseAddr("1.2.3.50"),
			wantUpPrefix: netip.MustParsePrefix("1.2.3.50/32"),
			wantBits:     32,
			wantString:   "1.2.3.50-1.2.3.50/32",
			wantToDB:     "(1.2.3.50/32,1.2.3.50/32)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loAddr := tc.in.LoAddr()
			loPrefix := tc.in.LoPrefix()
			upAddr := tc.in.UpAddr()
			upPrefix := tc.in.UpPrefix()
			bits := tc.in.Bits()
			gotString := tc.in.String()
			gotToDB := tc.in.ToDB()
			require.Equal(t, tc.wantLoAddr, loAddr)
			require.Equal(t, tc.wantLoPrefix, loPrefix)
			require.Equal(t, tc.wantUpAddr, upAddr)
			require.Equal(t, tc.wantUpPrefix, upPrefix)
			require.Equal(t, tc.wantBits, bits)
			require.Equal(t, tc.wantString, gotString)
			require.Equal(t, tc.wantToDB, gotToDB)
		})
	}
}

func TestAddrRangeFromDB(t *testing.T) {
	cases := []struct {
		name          string
		in            string
		want          AddrRange
		expectFailure bool
	}{
		{
			name: "standard example",
			in:   "(1.2.3.50/24,1.2.3.99/24)",
			want: MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
		},
		{
			name: "single address",
			in:   "(1.2.3.4/24,1.2.3.4/24)",
			want: MustParseAddrRange("1.2.3.4-1.2.3.4/24"),
		},
		{
			name: "23-bit mask",
			in:   "(1.2.3.50/23,1.2.3.99/23)",
			want: MustParseAddrRange("1.2.3.50-1.2.3.99/23"),
		},
		{
			name: "25-bit mask",
			in:   "(1.2.3.50/25,1.2.3.99/25)",
			want: MustParseAddrRange("1.2.3.50-1.2.3.99/25"),
		},
		{
			name: "0-bit mask",
			in:   "(1.2.3.50/0,1.2.3.99/0)",
			want: MustParseAddrRange("1.2.3.50-1.2.3.99/0"),
		},
		{
			name: "32-bit mask",
			in:   "(1.2.3.50/32,1.2.3.50/32)",
			want: MustParseAddrRange("1.2.3.50-1.2.3.50/32"),
		},
		{
			name:          "invalid address range",
			in:            "asdfgh",
			expectFailure: true,
		},
		{
			name:          "missing leading parenthesis",
			in:            "1.2.3.50/24,1.2.3.99/24)",
			expectFailure: true,
		},
		{
			name:          "missing trailing parenthesis",
			in:            "(1.2.3.50/24,1.2.3.99/24",
			expectFailure: true,
		},
		{
			name:          "missing comma",
			in:            "(1.2.3.50/24-1.2.3.99/24)",
			expectFailure: true,
		},
		{
			name:          "only one part",
			in:            "(1.2.3.50/24)",
			expectFailure: true,
		},
		{
			name:          "three parts",
			in:            "(1.2.3.50/24,1.2.3.51/24,1.2.3.99/24)",
			expectFailure: true,
		},
		{
			name:          "invalid lower address",
			in:            "(asdfgh,1.2.3.99/24)",
			expectFailure: true,
		},
		{
			name:          "invalid upper address",
			in:            "(1.2.3.50/24,asdfgh)",
			expectFailure: true,
		},
		{
			name: "missing masks (implicitly 32 bits)",
			in:   "(1.2.3.50,1.2.3.50)",
			want: MustParseAddrRange("1.2.3.50-1.2.3.50/32"),
		},
		{
			name: "missing mask in lower address (implicitly 32 bits)",
			in:   "(1.2.3.50,1.2.3.50/32)",
			want: MustParseAddrRange("1.2.3.50-1.2.3.50/32"),
		},
		{
			name: "missing mask in upper address (implicitly 32 bits)",
			in:   "(1.2.3.50/32,1.2.3.50)",
			want: MustParseAddrRange("1.2.3.50-1.2.3.50/32"),
		},
		{
			name:          "missing mask in lower address (implicitly 32 bits) - mask mismatch",
			in:            "(1.2.3.50,1.2.3.50/24)",
			expectFailure: true,
		},
		{
			name:          "missing mask in upper address (implicitly 32 bits) - mask mismatch",
			in:            "(1.2.3.50/24,1.2.3.50)",
			expectFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AddrRangeFromDB(tc.in)
			if tc.expectFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestToContainingSubnet(t *testing.T) {
	cases := []struct {
		name string
		in   AddrRange
		want netip.Prefix
	}{
		{
			name: "standard example",
			in:   MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
			want: netip.MustParsePrefix("1.2.3.0/24"),
		},
		{
			name: "single address",
			in:   MustParseAddrRange("1.2.3.4-1.2.3.4/24"),
			want: netip.MustParsePrefix("1.2.3.0/24"),
		},
		{
			name: "23-bit mask",
			in:   MustParseAddrRange("1.2.3.50-1.2.3.99/23"),
			want: netip.MustParsePrefix("1.2.2.0/23"),
		},
		{
			name: "23-bit mask again",
			in:   MustParseAddrRange("1.2.4.50-1.2.4.99/23"),
			want: netip.MustParsePrefix("1.2.4.0/23"),
		},
		{
			name: "25-bit mask",
			in:   MustParseAddrRange("1.2.3.50-1.2.3.99/25"),
			want: netip.MustParsePrefix("1.2.3.0/25"),
		},
		{
			name: "25-bit mask again",
			in:   MustParseAddrRange("1.2.3.150-1.2.3.199/25"),
			want: netip.MustParsePrefix("1.2.3.128/25"),
		},
		{
			name: "0-bit mask",
			in:   MustParseAddrRange("1.2.3.50-1.2.3.99/0"),
			want: netip.MustParsePrefix("0.0.0.0/0"),
		},
		{
			name: "32-bit mask",
			in:   MustParseAddrRange("1.2.3.50-1.2.3.50/32"),
			want: netip.MustParsePrefix("1.2.3.50/32"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.ToContainingSubnet()
			require.Equal(t, tc.want, got)
		})
	}
}

func TestContains(t *testing.T) {
	cases := []struct {
		name             string
		ran              AddrRange
		addr             netip.Prefix
		expectedContains bool
	}{
		{
			name:             "standard example",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
			addr:             netip.MustParsePrefix("1.2.3.51/24"),
			expectedContains: true,
		},
		{
			name:             "first address in range",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
			addr:             netip.MustParsePrefix("1.2.3.50/24"),
			expectedContains: true,
		},
		{
			name:             "last address in range",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
			addr:             netip.MustParsePrefix("1.2.3.99/24"),
			expectedContains: true,
		},
		{
			name:             "before range",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
			addr:             netip.MustParsePrefix("1.2.3.49/24"),
			expectedContains: false,
		},
		{
			name:             "after range",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
			addr:             netip.MustParsePrefix("1.2.3.100/24"),
			expectedContains: false,
		},
		{
			name:             "single address - contains",
			ran:              MustParseAddrRange("1.2.3.4-1.2.3.4/24"),
			addr:             netip.MustParsePrefix("1.2.3.4/24"),
			expectedContains: true,
		},
		{
			name:             "single address - before",
			ran:              MustParseAddrRange("1.2.3.4-1.2.3.4/24"),
			addr:             netip.MustParsePrefix("1.2.3.3/24"),
			expectedContains: false,
		},
		{
			name:             "single address - after",
			ran:              MustParseAddrRange("1.2.3.4-1.2.3.4/24"),
			addr:             netip.MustParsePrefix("1.2.3.5/24"),
			expectedContains: false,
		},
		{
			name:             "25-bit mask - contains",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/25"),
			addr:             netip.MustParsePrefix("1.2.3.51/25"),
			expectedContains: true,
		},
		{
			name:             "25-bit mask - before",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/25"),
			addr:             netip.MustParsePrefix("1.2.3.49/25"),
			expectedContains: false,
		},
		{
			name:             "25-bit mask - after",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/25"),
			addr:             netip.MustParsePrefix("1.2.3.100/25"),
			expectedContains: false,
		},
		{
			name:             "address with longer mask",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/24"),
			addr:             netip.MustParsePrefix("1.2.3.51/25"),
			expectedContains: false,
		},
		{
			name:             "address with shorter mask",
			ran:              MustParseAddrRange("1.2.3.50-1.2.3.99/25"),
			addr:             netip.MustParsePrefix("1.2.3.51/24"),
			expectedContains: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			contains := tc.ran.Contains(tc.addr)
			require.Equal(t, tc.expectedContains, contains)
		})
	}
}

func TestFirstAvailableAddress(t *testing.T) {
	success := func(addrRange AddrRange, usedAddrs []netip.Addr, expectedAddr netip.Prefix) {
		t.Helper()
		chosenAddr, err := addrRange.FirstAvailableAddress(usedAddrs)
		if err != nil {
			t.Fatalf("expected error to be nil, got %v", err)
		}
		if chosenAddr != expectedAddr {
			t.Fatalf("expected chosen address to be %s, got %s", expectedAddr, chosenAddr)
		}
	}

	failure := func(addrRange AddrRange, usedAddrs []netip.Addr) {
		t.Helper()
		chosenAddr, err := addrRange.FirstAvailableAddress(usedAddrs)
		if err == nil {
			t.Fatalf("expected non-nil error, got return value %s", chosenAddr)
		}
	}

	usedAddrs := []netip.Addr{}
	success(MustAddrRangeFromSubnet(netip.MustParsePrefix("10.10.10.0/28")), usedAddrs,
		netip.MustParsePrefix("10.10.10.1/28"))

	usedAddrs = []netip.Addr{netip.AddrFrom4([4]byte{10, 10, 10, 1}), netip.AddrFrom4([4]byte{10, 10, 10, 2})}
	success(MustAddrRangeFromSubnet(netip.MustParsePrefix("10.10.10.0/24")), usedAddrs,
		netip.MustParsePrefix("10.10.10.3/24"))

	usedAddrs = []netip.Addr{netip.AddrFrom4([4]byte{10, 10, 10, 4}), netip.AddrFrom4([4]byte{10, 10, 10, 1}), netip.AddrFrom4([4]byte{10, 10, 10, 2})}
	success(MustAddrRangeFromSubnet(netip.MustParsePrefix("10.10.10.0/24")), usedAddrs,
		netip.MustParsePrefix("10.10.10.3/24"))

	usedAddrs = []netip.Addr{netip.AddrFrom4([4]byte{10, 10, 10, 1}), netip.AddrFrom4([4]byte{10, 10, 10, 2})}
	failure(MustAddrRangeFromSubnet(netip.MustParsePrefix("10.10.10.0/30")), usedAddrs)

	usedAddrs = []netip.Addr{}
	// 10.10.10.1 up to 10.10.10.254
	for i := 1; i <= 254; i++ {
		usedAddrs = append(usedAddrs, netip.AddrFrom4([4]byte{10, 10, 10, byte(i)}))
	}
	failure(MustAddrRangeFromSubnet(netip.MustParsePrefix("10.10.10.0/24")), usedAddrs)

	addrRange := MustParseAddrRange("10.10.10.100-10.10.10.102/24")
	usedAddrs = []netip.Addr{netip.AddrFrom4([4]byte{10, 10, 10, 100}), netip.AddrFrom4([4]byte{10, 10, 10, 101})}
	success(addrRange, usedAddrs, netip.MustParsePrefix("10.10.10.102/24"))

	addrRange = MustParseAddrRange("10.10.10.100-10.10.10.102/24")
	usedAddrs = []netip.Addr{netip.AddrFrom4([4]byte{10, 10, 10, 100}), netip.AddrFrom4([4]byte{10, 10, 10, 101}), netip.AddrFrom4([4]byte{10, 10, 10, 102})}
	failure(addrRange, usedAddrs)

}
