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

// UserDataRequest is sent to the plugins by the manager.
type UserDataRequest struct {
	MachineSpec   clusterv1alpha1.MachineSpec
	KubeConfig    *clientcmdapi.Config
	CloudConfig   cloud.ConfigProvider
	CloudProvider string
	DNSIPs        []net.IP
}

// UserDataResponse will be responded.
type UserDataResponse struct {
	UserData string
	Err      string
}
