/*
Copyright 2019 The Machine Controller Authors.

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

//
// UserData plugin manager.
//

package manager

import (
	"encoding/json"
	"fmt"
	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/amzn2"
	"github.com/kubermatic/machine-controller/pkg/userdata/centos"
	"github.com/kubermatic/machine-controller/pkg/userdata/flatcar"
	userdataplugin "github.com/kubermatic/machine-controller/pkg/userdata/plugin"
	"github.com/kubermatic/machine-controller/pkg/userdata/rhel"
	"github.com/kubermatic/machine-controller/pkg/userdata/rockylinux"
	"github.com/kubermatic/machine-controller/pkg/userdata/sles"
	"github.com/kubermatic/machine-controller/pkg/userdata/ubuntu"
)

// Plugin looks for the plugin executable and calls it for
// each request.
type Plugin struct {
	provider userdataplugin.Provider
}

// newPlugin creates a new plugin manager. It starts the named
// binary and connects to it via net/rpc.
func newPlugin(os providerconfigtypes.OperatingSystem, debug bool) (*Plugin, error) {
	var provider userdataplugin.Provider
	switch os {
	case providerconfigtypes.OperatingSystemUbuntu:
		provider = new(ubuntu.Provider)
	case providerconfigtypes.OperatingSystemCentOS:
		provider = new(centos.Provider)
	case providerconfigtypes.OperatingSystemAmazonLinux2:
		provider = new(amzn2.Provider)
	case providerconfigtypes.OperatingSystemSLES:
		provider = new(sles.Provider)
	case providerconfigtypes.OperatingSystemRHEL:
		provider = new(rhel.Provider)
	case providerconfigtypes.OperatingSystemFlatcar:
		provider = new(flatcar.Provider)
	case providerconfigtypes.OperatingSystemRockyLinux:
		provider = new(rockylinux.Provider)
	}

	p := &Plugin{
		provider: provider,
	}

	return p, nil
}

// UserData retrieves the user data of the given resource via
// plugin handling the communication.
func (p *Plugin) UserData(req plugin.UserDataRequest) (string, error) {
	out, err := p.provider.UserData(req)
	if err != nil {
		return "", fmt.Errorf("FAILED TO PROVIDE USERDATA: %w", err)
	}

	fmt.Println(string(out))

	var resp plugin.UserDataResponse
	err = json.Unmarshal([]byte(out), &resp)
	if err != nil {
		return "", fmt.Errorf("FAILED TO UNMARSHALL: %w", err)
	}
	if resp.Err != "" {
		return "", fmt.Errorf("%s", resp.Err)
	}
	return resp.UserData, nil
}
