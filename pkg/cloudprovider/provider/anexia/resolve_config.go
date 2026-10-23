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

package anexia

import (
	"context"
	"fmt"

	"github.com/anexia/go-anxsdk/v1/vsphere"
	"go.uber.org/zap"

	anxtypes "k8c.io/machine-controller/sdk/cloudprovider/anexia"
)

// resolvedDisk contains the resolved values from types.RawDisk.
type resolvedDisk struct {
	anxtypes.RawDisk

	PerformanceType string
}

// resolvedNetwork contains the resolved values from types.RawNetwork.
type resolvedNetwork struct {
	VlanID string

	// List of prefixes to each reserve an IP address from.
	//
	// Legacy compatibility: may contain an empty string as entry to reserve an IP address from the given VLAN instead of a specific prefix.
	Prefixes []string

	BandwidthLimit int
}

// resolvedConfig contains the resolved values from types.RawConfig.
type resolvedConfig struct {
	LocationID string
	TemplateID string

	Disks               []resolvedDisk
	Networks            []resolvedNetwork
	SSHPublicKeys       []string
	CPUs                int
	CPUPerformanceType  string
	DiskSize            int
	DiskPerformanceType string
	Memory              int

	AvailabilityZone string
}

func (p *provider) resolveTemplateID(ctx context.Context, provisioningAPI vsphere.ProvisioningAPI, config anxtypes.RawConfig, locationID string) (string, error) {
	templateName, err := p.configVarResolver.GetStringValue(config.Template)
	if err != nil {
		return "", fmt.Errorf("failed to get 'template': %w", err)
	}

	templateBuild, err := p.configVarResolver.GetStringValue(config.TemplateBuild)
	if err != nil {
		return "", fmt.Errorf("failed to get 'templateBuild': %w", err)
	}

	template, err := provisioningAPI.FindNamedTemplate(ctx, locationID, templateName, templateBuild)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve named template: %w", err)
	}

	return template.ID, nil
}

func (p *provider) resolveNetworkConfig(log *zap.SugaredLogger, config anxtypes.RawConfig) (*[]resolvedNetwork, error) {
	ret := make([]resolvedNetwork, len(config.Networks))
	for netIndex, net := range config.Networks {
		vlanID, err := p.configVarResolver.GetStringValue(net.VlanID)
		if err != nil {
			return nil, fmt.Errorf("failed to get 'vlanID' for network %v: %w", netIndex, err)
		}

		prefixes := make([]string, len(net.PrefixIDs))
		for prefixIndex, prefix := range net.PrefixIDs {
			prefixID, err := p.configVarResolver.GetStringValue(prefix)
			if err != nil {
				return nil, fmt.Errorf("failed to get 'prefixID' for network %v, prefix %v: %w", netIndex, prefixIndex, err)
			}

			prefixes[prefixIndex] = prefixID
		}

		bandwidthLimit, err := p.resolveBandwidthLimitConfig(net)
		if err != nil {
			return nil, fmt.Errorf("failed to parse bandwidth limit: %w", err)
		}

		ret[netIndex] = resolvedNetwork{
			VlanID:         vlanID,
			Prefixes:       prefixes,
			BandwidthLimit: bandwidthLimit,
		}
	}

	return &ret, nil
}

func (p *provider) resolveBandwidthLimitConfig(config anxtypes.RawNetwork) (int, error) {
	switch config.BandwidthLimit {
	case 0, 100, 1_000, 10_000:
		return config.BandwidthLimit, nil
	default:
		return 0, fmt.Errorf("bandwidthLimit %d invalid, needs to be one of: 0, 100, 1000, 10000", config.BandwidthLimit)
	}
}

func (p *provider) resolveDiskConfig(_ *zap.SugaredLogger, config anxtypes.RawConfig) (*[]resolvedDisk, error) {
	ret := make([]resolvedDisk, len(config.Disks))

	for idx, disk := range config.Disks {
		performanceType, err := p.configVarResolver.GetStringValue(disk.PerformanceType)
		if err != nil {
			return nil, fmt.Errorf("failed to get 'performanceType' of disk %v: %w", idx, err)
		}

		ret[idx] = resolvedDisk{
			RawDisk:         disk,
			PerformanceType: performanceType,
		}
	}

	return &ret, nil
}

func (p *provider) resolveConfig(ctx context.Context, log *zap.SugaredLogger, config anxtypes.RawConfig) (*resolvedConfig, error) {
	var err error
	ret := resolvedConfig{
		CPUs:                config.CPUs,
		CPUPerformanceType:  config.CPUPerformanceType,
		DiskSize:            config.DiskSize,
		DiskPerformanceType: config.DiskPerformanceType,
		Memory:              config.Memory,
		AvailabilityZone:    config.AvailabilityZone,
	}

	ret.LocationID, err = p.configVarResolver.GetStringValue(config.LocationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get 'locationID': %w", err)
	}

	ret.TemplateID, err = p.configVarResolver.GetStringValue(config.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get 'templateID': %w", err)
	}

	diskConfig, err := p.resolveDiskConfig(log, config)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve disk config: %w", err)
	}
	ret.Disks = *diskConfig

	networkConfig, err := p.resolveNetworkConfig(log, config)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve network config: %w", err)
	}
	ret.Networks = *networkConfig

	// when "templateID" is not set, we expect "template" to be
	if ret.TemplateID == "" {
		provisioningClient := getSDKClient(nil).V1().VSphere().Provisioning()

		templateID, err := p.resolveTemplateID(ctx, provisioningClient, config, ret.LocationID)
		if err != nil {
			return nil, fmt.Errorf("failed retrieving template id from named template: %w", err)
		}

		ret.TemplateID = templateID
	}

	return &ret, nil
}
