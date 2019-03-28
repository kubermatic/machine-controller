//
// Core UserData plugin.
//

package plugin

import (
	"net"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
)

// PingRequest is sent if the manager restarts to test if a
// plugin is running.
type PingRequest struct {
}

// PingResponse will be responded.
type PingResponse struct {
	Executable string
}

// UserDataRequest is sent to the plugins by the manager.
type UserDataRequest struct {
	MachineSpec           clusterv1alpha1.MachineSpec
	KubeConfig            *clientcmdapi.Config
	CloudConfig           cloud.ConfigProvider
	DNSIPs                []net.IP
	ExternalCloudProvider bool
}

// UserDataResponse will be responded.
type UserDataResponse struct {
	UserData string
	Err      string
}
