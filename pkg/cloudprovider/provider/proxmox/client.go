/*
Copyright 2022 The Machine Controller Authors.

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

package proxmox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	proxmoxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/proxmox/types"
)

const (
	taskTimeout       = 300
	exitStatusSuccess = "OK"
)

type ClientSet struct {
	*proxmox.Client
}

func GetClientSet(config *Config) (*ClientSet, error) {
	if config == nil {
		return nil, errors.New("no configuration passed")
	}

	if config.UserID == "" {
		return nil, errors.New("no user_id specified")
	}

	if config.Token == "" {
		return nil, errors.New("no token specificed")
	}

	if config.Endpoint == "" {
		return nil, errors.New("no endpoint specified")
	}

	client, err := proxmox.NewClient(config.Endpoint, nil, &tls.Config{InsecureSkipVerify: config.TLSInsecure}, config.ProxyURL, taskTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not initiate proxmox client: %w", err)
	}

	client.SetAPIToken(config.UserID, config.Token)

	return &ClientSet{client}, nil
}

func (c ClientSet) getVMRefByName(name string) (*proxmox.VmRef, error) {
	vmr, err := c.GetVmRefByName(name)
	if err != nil {
		if err.Error() == fmt.Sprintf("vm '%s' not found", name) {
			return nil, cloudprovidererrors.ErrInstanceNotFound
		}
		return nil, err
	}

	return vmr, nil
}

func (c ClientSet) getNodeList() (*proxmoxtypes.NodeList, error) {
	nodeList, err := c.GetNodeList()
	if err != nil {
		return nil, fmt.Errorf("cannot fetch nodes from cluster: %w", err)
	}

	var nl *proxmoxtypes.NodeList

	nodeListJSON, err := json.Marshal(nodeList)
	if err != nil {
		return nil, fmt.Errorf("marshalling nodeList to JSON: %w", err)
	}
	err = json.Unmarshal(nodeListJSON, &nl)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling JSON to NodeList: %w", err)
	}

	return nl, nil
}

func (c ClientSet) checkTemplateExists(vmID int) (bool, error) {
	vmInfo, err := c.GetVmInfo(proxmox.NewVmRef(vmID))
	if err != nil {
		return false, fmt.Errorf("failed to retrieve info for VM template %d", vmID)
	}

	return vmInfo["template"] == 1, nil
}

func (c ClientSet) getIPsByVMRef(vmr *proxmox.VmRef) (map[string]corev1.NodeAddressType, error) {
	addresses := map[string]corev1.NodeAddressType{}
	netInterfaces, err := c.GetVmAgentNetworkInterfaces(vmr)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("failed to get network interfaces: %v", err),
		}
	}
	for _, netIf := range netInterfaces {
		if netIf.Name == "lo" {
			continue
		}
		for _, ipAddr := range netIf.IPAddresses {
			if len(ipAddr) > 0 {
				ip := ipAddr.String()
				addresses[ip] = corev1.NodeInternalIP
			}
		}
	}

	return addresses, nil
}

func (c ClientSet) copyUserdata(ctx context.Context, node, localStoragePath, userID, userdata, privateKey string, vmID int) (string, error) {
	nodeIP, err := c.getNodeIP(node)
	if err != nil {
		return "", fmt.Errorf("failed to get node IP: %w", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return "", fmt.Errorf("could not parse private key: %w", err)
	}

	username := strings.Split(userID, "@")[0]
	sshConfig := ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	client, err := ssh.Dial("tcp", nodeIP+":22", &sshConfig)
	if err != nil {
		return "", fmt.Errorf("unable to connect: %w", err)
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("failed to create SFTP client: %w", err)
	}

	filePath := filepath.Join("snippets", fmt.Sprintf("userdata-%d.yml", vmID))
	remoteFilePath := filepath.Join(localStoragePath, filePath)
	remoteFile, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()

	_, err = remoteFile.ReadFrom(strings.NewReader(userdata))

	return filePath, err
}

func (c ClientSet) getNodeIP(node string) (string, error) {
	var response map[string]interface{}
	var devices proxmoxtypes.NodeNetworkDeviceList
	err := c.GetJsonRetryable(fmt.Sprintf("/nodes/%s/network", node), &response, 3)
	if err != nil {
		return "", fmt.Errorf("could not get node network data: %w", err)
	}

	// JSON roundtrip to transform map[string]interface{} to struct
	devicesJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("marshalling response to JSON: %w", err)
	}
	err = json.Unmarshal(devicesJSON, &devices)
	if err != nil {
		return "", fmt.Errorf("unmarshalling JSON to NodeNetworkDeviceList: %w", err)
	}

	sort.Slice(devices.Data, func(i, j int) bool {
		return devices.Data[i].Priority < devices.Data[j].Priority
	})

	for _, d := range devices.Data {
		if d.Address != nil {
			return strings.Split(*d.Address, "/")[0], nil
		}
	}

	return "", fmt.Errorf("could not retrieve IP for node %q", node)
}

func (c ClientSet) selectNode(cpuCores, memoryMB int) (string, error) {
	nodeList, err := c.getNodeList()
	if err != nil {
		return "", fmt.Errorf("no nodes to select from: %w", err)
	}

	// For the first tech demo just pick a random available node. Later more
	// sophisticated approaches may be posslibe like avoiding overutilized
	// nodes or round robin.

	return nodeList.Data[rand.Intn(len(nodeList.Data))].Name, nil
}
