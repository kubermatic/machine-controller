//
// UserData plugin manager.
//

package manager

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	"github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

const (
	// envPluginDir names the environment variable containing
	// a user defined location of the plugins.
	envPluginDir = "MACHINE_CONTROLLER_USERDATA_PLUGIN_DIR"

	// pluginPrefix has to be the prefix of all plugin filenames.
	pluginPrefix = "machine-controller-userdata-"
)

// Plugin looks for the plugin executable and calls it for
// each request.
type Plugin struct {
	os      providerconfig.OperatingSystem
	debug   bool
	command string
}

// newPlugin creates a new plugin manager. It starts the named
// binary and connects to it via net/rpc.
func newPlugin(os providerconfig.OperatingSystem, debug bool) (*Plugin, error) {
	p := &Plugin{
		os:    os,
		debug: debug,
	}
	if err := p.findPlugin(); err != nil {
		return nil, err
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
	externalCloudProvider bool,
) (string, error) {
	// Prepare command.
	var argv []string
	if p.debug {
		argv = append(argv, "-debug")
	}
	cmd := exec.Command(p.command, argv...)
	// Set environment.
	req := plugin.UserDataRequest{
		MachineSpec:           spec,
		KubeConfig:            kubeconfig,
		CloudConfig:           ccProvider,
		DNSIPs:                clusterDNSIPs,
		ExternalCloudProvider: externalCloudProvider,
	}
	reqj, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	cmd.Env = []string{
		fmt.Sprintf("%s=%s", plugin.EnvRequest, plugin.EnvUserDataRequest),
		fmt.Sprintf("%s=%s", plugin.EnvUserDataRequest, string(reqj)),
	}
	// Execute command.
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("output error: %v", err)
		return "", fmt.Errorf("failed to execute command %q: output: %q error: %q", p.command, string(out), err)
	}
	log.Printf("output: %v", out)
	var resp plugin.UserDataResponse
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
func (p *Plugin) findPlugin() error {
	filename := pluginPrefix + string(p.os)
	log.Printf("looking for plugin '%s'", filename)
	// Create list to search in.
	var dirs []string
	envDir := os.Getenv(envPluginDir)
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
		log.Printf("checking '%s'", command)
		_, err := os.Stat(command)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("error when looking for %q: %v", command, err)
		}
		p.command = command
		log.Printf("found '%s'", command)
		return nil
	}
	log.Printf("did not find '%s'", filename)
	return ErrPluginNotFound
}
