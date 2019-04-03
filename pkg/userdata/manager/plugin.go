//
// UserData plugin manager.
//

package manager

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/apis/userdata"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

const (
	// pluginPrefix has to be the prefix of all plugin filenames.
	pluginPrefix = "machine-controller-userdata-"
)

// Plugin looks for the plugin executable and calls it for
// each request.
type Plugin struct {
	debug   bool
	command string
}

// newPlugin creates a new plugin manager. It starts the named
// binary and connects to it via net/rpc.
func newPlugin(os providerconfig.OperatingSystem, debug bool) (*Plugin, error) {
	p := &Plugin{
		debug: debug,
	}
	if err := p.findPlugin(string(os)); err != nil {
		return nil, err
	}
	return p, nil
}

// UserData retrieves the user data of the given resource via
// plugin handling the communication.
func (p *Plugin) UserData(
	spec clusterv1alpha1.MachineSpec,
	kubeconfig *clientcmdapi.Config,
	cloudConfig string,
	cloudProviderName string,
	clusterDNSIPs []net.IP,
	externalCloudProvider bool,
) (string, error) {
	// Prepare command.
	var argv []string
	if p.debug {
		argv = append(argv, "-debug")
	}
	cmd := exec.Command(p.command, argv...)
	// Set environment.
	req := userdata.UserDataRequest{
		MachineSpec:           spec,
		KubeConfig:            kubeconfig,
		CloudProviderName:     cloudProviderName,
		CloudConfig:           cloudConfig,
		DNSIPs:                clusterDNSIPs,
		ExternalCloudProvider: externalCloudProvider,
	}
	reqj, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	cmd.Env = []string{
		fmt.Sprintf("%s=%s", userdata.EnvRequest, userdata.EnvUserDataRequest),
		fmt.Sprintf("%s=%s", userdata.EnvUserDataRequest, string(reqj)),
	}
	// Execute command.
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command %q: output: %q error: %q", p.command, string(out), err)
	}
	var resp userdata.UserDataResponse
	err = json.Unmarshal(out, &resp)
	if err != nil {
		return "", err
	}
	if resp.Err != "" {
		return "", fmt.Errorf("%s", resp.Err)
	}
	return resp.UserData, nil
}

// findPlugin tries to find the executable of the plugin.
func (p *Plugin) findPlugin(name string) error {
	filename := pluginPrefix + name
	glog.Infof("looking for plugin '%s'", filename)
	// Create list to search in.
	var dirs []string
	envDir := os.Getenv(userdata.EnvPluginDir)
	if envDir != "" {
		dirs = append(dirs, envDir)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ownDir, _ := filepath.Split(executable)
	ownDir, err = filepath.Abs(ownDir)
	if err != nil {
		return err
	}
	dirs = append(dirs, ownDir)
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	dirs = append(dirs, workingDir)
	path := os.Getenv("PATH")
	pathDirs := strings.Split(path, string(os.PathListSeparator))
	dirs = append(dirs, pathDirs...)
	// Now take a look.
	for _, dir := range dirs {
		command := dir + string(os.PathSeparator) + filename
		glog.Infof("checking '%s'", command)
		_, err := os.Stat(command)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("error when looking for %q: %v", command, err)
		}
		p.command = command
		glog.Infof("found '%s'", command)
		return nil
	}
	glog.Errorf("did not find '%s'", filename)
	return ErrPluginNotFound
}
