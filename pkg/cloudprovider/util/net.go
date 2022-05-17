/*
Copyright 2021 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net"
)

const (
	ErrIPv6OnlyUnsupported  = "IPv6 only network family not supported yet"
	ErrUnknownNetworkFamily = "Unknown IP family %q only IPv4,IPv6,IPv4+IPv6 are valid values"
)

func CIDRToIPAndNetMask(ipv4 string) (string, string, int, error) {
	ip, ipNet, err := net.ParseCIDR(ipv4)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to parse CIDR prefix: %w", err)
	}

	if len(ipNet.Mask) != 4 {
		return "", "", 0, errors.New("inappropriate netmask length, netmask should be 4 bytes")
	}
	size, _ := ipNet.Mask.Size()

	netmask := fmt.Sprintf("%d.%d.%d.%d", ipNet.Mask[0], ipNet.Mask[1], ipNet.Mask[2], ipNet.Mask[3])
	return ip.String(), netmask, size, nil
}

// GenerateRandMAC generates a random unicast and locally administered MAC address.
func GenerateRandMAC() (net.HardwareAddr, error) {
	buf := make([]byte, 6)
	var mac net.HardwareAddr

	_, err := rand.Read(buf)
	if err != nil {
		return mac, err
	}

	// Set locally administered addresses bit and reset multicast bit
	buf[0] = (buf[0] | 0x02) & 0xfe
	mac = append(mac, buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])

	return mac, nil
}

// IPFamily IPv4 | IPv6 | IPv4+IPv6.
type IPFamily string

const (
	Unspecified IPFamily = "" // interpreted as IPv4
	IPv4        IPFamily = "IPv4"
	IPv6        IPFamily = "IPv6"
	DualStack   IPFamily = "IPv4+IPv6"
)

// IsLinkLocal checks if given ip address is link local..
func IsLinkLocal(ipAddr string) bool {
	addr := net.ParseIP(ipAddr)
	return addr.IsLinkLocalMulticast() || addr.IsLinkLocalUnicast()
}
