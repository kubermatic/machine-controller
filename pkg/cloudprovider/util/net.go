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
	"errors"
	"fmt"
	net2 "k8s.io/utils/net"
	"net"
)

func CIDRToIPAndNetMask(ipv4 string) (string, string, int, error) {
	ip, ipNet, err := net.ParseCIDR(ipv4)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to parse CIDR prefix: %v", err)
	}

	if len(ipNet.Mask) != 4 {
		return "", "", 0, errors.New("inappropriate netmask length, netmask should be 4 bytes")
	}
	size, _ := ipNet.Mask.Size()

	netmask := fmt.Sprintf("%d.%d.%d.%d", ipNet.Mask[0], ipNet.Mask[1], ipNet.Mask[2], ipNet.Mask[3])
	return ip.String(), netmask, size, nil
}

type IPVersion int

const (
	IPv4 = iota
	IPv6
)

// ContainsCIDR checks if cidrs contains a CIDR block of given version (IPv4 or IPv6).
func ContainsCIDR(cidrs []string, version IPVersion) bool {
	f := func(string) bool { return false }

	switch version {
	case IPv4:
		f = net2.IsIPv4CIDRString
	case IPv6:
		f = net2.IsIPv6CIDRString
	}

	for _, cidr := range cidrs {
		if f(cidr) {
			return true
		}
	}

	return false
}
