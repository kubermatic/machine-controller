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

package net

import (
	gonet "net"
)

const (
	ErrIPv6OnlyUnsupported  = "IPv6-only network family not supported yet"
	ErrUnknownNetworkFamily = "unknown IP family %q, only IPv4,IPv6,IPv4+IPv6 are valid values"
)

// IPFamily IPv4 | IPv6 | IPv4+IPv6.
type IPFamily string

const (
	IPFamilyUnspecified IPFamily = ""          // interpreted as IPv4
	IPFamilyIPv4        IPFamily = "IPv4"      // IPv4 only
	IPFamilyIPv6        IPFamily = "IPv6"      // IPv6 only
	IPFamilyIPv4IPv6    IPFamily = "IPv4+IPv6" // dualstack with IPv4 as primary
	IPFamilyIPv6IPv4    IPFamily = "IPv6+IPv4" // dualstack with IPv6 as primary
)

func (f IPFamily) HasIPv6() bool {
	return f == IPFamilyIPv6 || f == IPFamilyIPv4IPv6 || f == IPFamilyIPv6IPv4
}

func (f IPFamily) HasIPv4() bool {
	return f == IPFamilyUnspecified || f == IPFamilyIPv4 || f == IPFamilyIPv4IPv6 || f == IPFamilyIPv6IPv4
}

func (f IPFamily) IsDualstack() bool {
	return f == IPFamilyIPv4IPv6 || f == IPFamilyIPv6IPv4
}

// IsLinkLocal checks if given ip address is link local..
func IsLinkLocal(ipAddr string) bool {
	addr := gonet.ParseIP(ipAddr)
	return addr.IsLinkLocalMulticast() || addr.IsLinkLocalUnicast()
}
