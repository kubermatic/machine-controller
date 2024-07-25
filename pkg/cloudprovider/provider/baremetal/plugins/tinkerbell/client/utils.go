/*
Copyright 2024 The Machine Controller Authors.

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

package client

import (
	"fmt"
	"net"
	"strings"

	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
)

func convertNetmaskToCIDR(ip *tinkv1alpha1.IP) string {
	mask := net.IPMask(net.ParseIP(ip.Netmask).To4())
	length, _ := mask.Size()

	cidr := ""
	parts := strings.Split(ip.Address, ".")
	for i := 0; i < len(parts); i++ {
		cidr += parts[i] + "."
	}
	cidr = strings.TrimSuffix(cidr, ".")

	return fmt.Sprintf("%s/%v", cidr, length)
}
