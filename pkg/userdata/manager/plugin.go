//
// Userdata manager for plugin handling from machine controller side.
//

package manager

import (
	"net"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
)

// Plugin manages the communication to one plugin. It is instantiated
// by the manager based on the directory scanning.
type Plugin struct {
	filename string
	port     int
	os       providerconfig.OperatingSystem
}

// newPlugin creates a new plugin manager. It starts the named
// binary and connects to it via gRPC.
func newPlugin(filename string, port int) (*Plugin, error) {
	p := &Plugin{
		filename: filename,
		port:     port,
	}
	return p, nil
}

// OperatingSystem returns the operating system this plugin is
// responsible for.
func (p *Plugin) OperatingSystem() providerconfig.OperatingSystem {
	return p.os
}

// UserData retrieves the user data of the given resource via
// plugin handling the communication.
func (p *Plugin) UserData(
	spec clusterv1alpha1.MachineSpec,
	kubeconfig *clientcmdapi.Config,
	ccProvider cloud.ConfigProvider,
	clusterDNSIPs []net.IP,
) (string, error) {
	return "", nil
}
