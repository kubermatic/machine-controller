//
// Core UserData plugin.
//

// Package plugin provides the communication net/rpc types
// as well as the data exchange types. Both then have to
// be used by the plugin implementations.
package plugin

import (
	"net"
	"net/http"
	"net/rpc"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
)

const (
	// RPCPath is the path for the RPC handler.
	RPCPath = "/machine-controller-plugin-rpc"

	// DebugPath is the path for the RPC debug handler.
	DebugPath = "/machine-controller-plugin-debug"
)

// Provider defines the interface each plugin has to implement
// for the retrieval of the userdata based on the given arguments.
type Provider interface {
	UserData(
		spec clusterv1alpha1.MachineSpec,
		kubeconfig *clientcmdapi.Config,
		ccProvider cloud.ConfigProvider,
		clusterDNSIPs []net.IP,
		externalCloudProvider bool,
	) (string, error)
}

// Handler cares dispatching of the RPC calls to the given Provider.
type Handler struct {
	provider Provider
}

// UserData receives the RPC message and calls the provider.
func (h *Handler) UserData(req *UserDataRequest, resp *UserDataResponse) error {
	userData, err := h.provider.UserData(
		req.MachineSpec,
		req.KubeConfig,
		req.CloudConfig,
		req.DNSIPs,
		req.ExternalCloudProvider,
	)
	resp.UserData = userData
	if err != nil {
		resp.Err = err.Error()
	}
	return nil
}

// Plugin implements the RPC server for the individual plugins. Those
// got to pass their individual userdata providers as well as their
// Unix socket address and debug flag their executable receives by the
// plugin manager.
type Plugin struct {
	handler  *Handler
	address  string
	debug    bool
	listener net.Listener
	server   *rpc.Server
}

// New creates a new plugin. Debug flag is not yet handled.
func New(provider Provider, address string, debug bool) *Plugin {
	p := &Plugin{
		handler: &Handler{provider},
		address: address,
		debug:   debug,
		server:  rpc.NewServer(),
	}
	p.server.HandleHTTP(RPCPath, DebugPath)
	p.server.RegisterName("Plugin", p.handler)
	return p
}

// Start starts the plugin and blocks.
func (p *Plugin) Start() error {
	l, err := net.Listen("unix", p.address)
	if err != nil {
		return err
	}
	p.listener = l
	return http.Serve(p.listener, nil)
}

// Stop closes open network listeners.
func (p *Plugin) Stop() error {
	return p.listener.Close()
}
