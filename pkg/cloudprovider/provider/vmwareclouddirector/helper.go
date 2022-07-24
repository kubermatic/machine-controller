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

package vmwareclouddirector

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	vcdapitypes "github.com/vmware/go-vcloud-director/v2/types/v56"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	"k8s.io/utils/pointer"
)

var internalDiskBusTypes = map[string]string{
	"ide":         "1",
	"parallel":    "3",
	"sas":         "4",
	"paravirtual": "5",
	"sata":        "6",
	"nvme":        "7",
}

func getComputePolicy(name string, policies []*govcd.VdcComputePolicy) *govcd.VdcComputePolicy {
	for _, policy := range policies {
		if policy.VdcComputePolicy == nil {
			continue
		}
		if policy.VdcComputePolicy.Name == name || policy.VdcComputePolicy.ID == name {
			return policy
		}
	}
	return nil
}

func createVM(client *Client, machine *clusterv1alpha1.Machine, c *Config, org *govcd.Org, vdc *govcd.Vdc, vapp *govcd.VApp) error {
	// 1. We need the template HREF for the VM.
	catalog, err := org.GetCatalogByNameOrId(c.Catalog, true)
	if err != nil {
		return fmt.Errorf("failed to get catalog '%s': %w", c.Catalog, err)
	}

	// Catalog item can be a vApp template OVA or media ISO file.
	catalogItem, err := catalog.GetCatalogItemByNameOrId(c.Template, true)
	if err != nil {
		return fmt.Errorf("failed to get catalog item '%s' in catalog '%s': %w", c.Template, c.Catalog, err)
	}

	vAppTemplate, err := catalogItem.GetVAppTemplate()
	if err != nil {
		return fmt.Errorf("failed to get vApp template '%s' in catalog '%s': %w", c.Template, c.Catalog, err)
	}

	templateHref := vAppTemplate.VAppTemplate.HREF
	if vAppTemplate.VAppTemplate.Children != nil && len(vAppTemplate.VAppTemplate.Children.VM) != 0 {
		templateHref = vAppTemplate.VAppTemplate.Children.VM[0].HREF
	}

	// 2. Retrieve Sizing and Placement Compute Policy if required.
	computePolicy := vcdapitypes.ComputePolicy{}
	if c.SizingPolicy != nil || c.PlacementPolicy != nil {
		allPolicies, err := org.GetAllVdcComputePolicies(url.Values{})
		if err != nil {
			return fmt.Errorf("failed to get template all VDC compute policies: %w", err)
		}

		if c.SizingPolicy != nil && *c.SizingPolicy != "" {
			sizingPolicy := getComputePolicy(*c.SizingPolicy, allPolicies)
			if sizingPolicy == nil {
				return fmt.Errorf("sizing policy '%s' doesn't exist", *c.SizingPolicy)
			}
			computePolicy.VmSizingPolicy = &vcdapitypes.Reference{
				HREF: sizingPolicy.VdcComputePolicy.ID,
			}
		}

		if c.PlacementPolicy != nil && *c.PlacementPolicy != "" {
			placementPolicy := getComputePolicy(*c.PlacementPolicy, allPolicies)
			if placementPolicy == nil {
				return fmt.Errorf("placement policy '%s' doesn't exist", *c.PlacementPolicy)
			}
			computePolicy.VmPlacementPolicy = &vcdapitypes.Reference{
				HREF: placementPolicy.VdcComputePolicy.ID,
			}
		}
	}

	// 3. Retrieve Storage Profile
	storageProfileRef := vcdapitypes.Reference{}
	if c.StorageProfile != nil && *c.StorageProfile != defaultStorageProfile {
		for _, sp := range vdc.Vdc.VdcStorageProfiles.VdcStorageProfile {
			if sp.Name == *c.StorageProfile || sp.ID == *c.StorageProfile {
				storageProfileRef = vcdapitypes.Reference{HREF: sp.HREF, Name: sp.Name, ID: sp.ID}
				break
			}
		}
		if storageProfileRef.HREF == "" {
			if err != nil {
				return fmt.Errorf("failed to get storage profile '%s': %w", *c.StorageProfile, err)
			}
		}
	}

	// 4. At this point we are ready to create our initial VMs.
	//
	// Multiple API calls to re-compose the vApp are handled in a synchronous manner, where each request has to wait
	// for the previous request to complete. This can cause a huge overhead in terms of time.
	//
	// It is not possible to customize compute, disk and network for a VM at initial creation time when we are using templates. So we rely on
	// vApp re-composition to apply the needed customization, performed at later stages.
	vAppRecomposition := &types.ReComposeVAppParams{
		Ovf:         types.XMLNamespaceOVF,
		Xsi:         types.XMLNamespaceXSI,
		Xmlns:       types.XMLNamespaceVCloud,
		Deploy:      false,
		Name:        vapp.VApp.Name,
		PowerOn:     false,
		Description: vapp.VApp.Description,
		SourcedItem: &types.SourcedCompositionItemParam{
			Source: &types.Reference{
				HREF: templateHref,
				Name: machine.Name,
			},
			InstantiationParams: &types.InstantiationParams{
				NetworkConnectionSection: &vcdapitypes.NetworkConnectionSection{
					NetworkConnection: []*vcdapitypes.NetworkConnection{
						{
							Network:                 c.Network,
							NeedsCustomization:      false,
							IsConnected:             true,
							IPAddressAllocationMode: string(c.IPAllocationMode),
							NetworkAdapterType:      "VMXNET3",
						},
					},
				},
			},
		},
		AllEULAsAccepted: true,
	}

	// Add storage profile
	if storageProfileRef.HREF != "" {
		vAppRecomposition.SourcedItem.StorageProfile = &storageProfileRef
	}

	// Add compute policy
	if computePolicy.HREF != "" {
		vAppRecomposition.SourcedItem.ComputePolicy = &computePolicy
	}

	apiEndpoint, err := url.Parse(vapp.VApp.HREF)
	if err != nil {
		return fmt.Errorf("error getting vapp href '%s': %w", c.Auth.URL, err)
	}
	apiEndpoint.Path = path.Join(apiEndpoint.Path, "action/recomposeVApp")

	task, err := client.VCDClient.Client.ExecuteTaskRequest(apiEndpoint.String(), http.MethodPost,
		types.MimeRecomposeVappParams, "error instantiating a new VM: %s", vAppRecomposition)
	if err != nil {
		return fmt.Errorf("unable to execute API call to create VM: %w", err)
	}

	// Wait for VM to be created this should take around 1-3 minutes
	if err = task.WaitTaskCompletion(); err != nil {
		return fmt.Errorf("error waiting for VM creation task to complete: %w", err)
	}
	return nil
}

func recomposeComputeAndDisk(config *Config, vm *govcd.VM) (*govcd.VM, error) {
	needsComputeRecomposition := false
	needsDiskRecomposition := false
	// Perform compute recomposition if SizingPolicy was not specified.
	vmSpecSection := vm.VM.VmSpecSection
	if config.SizingPolicy == nil || *config.SizingPolicy == "" {
		vmSpecSection.MemoryResourceMb.Configured = config.MemoryMB
		vmSpecSection.NumCpus = pointer.Int(int(config.CPUs))
		vmSpecSection.NumCoresPerSocket = pointer.Int(int(config.CPUCores))
		needsComputeRecomposition = true
	}

	// Perform disk recomposition if required.
	if vmSpecSection.DiskSection != nil {
		for i, internalDisk := range vmSpecSection.DiskSection.DiskSettings {
			// We are only concerned with template disk and not named/independent disks.
			if internalDisk.Disk == nil {
				if config.DiskSizeGB != nil && *config.DiskSizeGB > 0 {
					vmSpecSection.DiskSection.DiskSettings[i].SizeMb = (*config.DiskSizeGB) * 1024
					needsDiskRecomposition = true
				}
				if config.DiskIOPS != nil && *config.DiskIOPS > 0 {
					vmSpecSection.DiskSection.DiskSettings[i].Iops = pointer.Int64(*config.DiskIOPS)
					needsDiskRecomposition = true
				}
				if config.DiskBusType != nil && *config.DiskBusType != "" {
					vmSpecSection.DiskSection.DiskSettings[i].AdapterType = internalDiskBusTypes[*config.DiskBusType]
					needsDiskRecomposition = true
				}
			}
		}
	}

	if !needsDiskRecomposition {
		// Update treats same values as changes and fails. Although if set to nil, it assumes that no changes are required for this field.
		vmSpecSection.DiskSection = nil
	}

	var err error
	// Execute disk and compute recomposition on our VM
	if needsComputeRecomposition || needsDiskRecomposition {
		description := vm.VM.Description
		vm, err = vm.UpdateVmSpecSection(vmSpecSection, description)
		if err != nil {
			return nil, fmt.Errorf("error updating VM spec section: %w", err)
		}
	}
	return vm, nil
}

func setUserData(userdata string, vm *govcd.VM, isFlatcar bool) error {
	userdataBase64 := base64.StdEncoding.EncodeToString([]byte(userdata))
	props := map[string]string{
		"disk.enableUUID": "1",
		"instance-id":     vm.VM.Name,
	}

	if isFlatcar {
		props["guestinfo.ignition.config.data"] = userdataBase64
		props["guestinfo.ignition.config.data.encoding"] = "base64"
	} else {
		props["user-data"] = userdataBase64
	}

	vmProperties := &vcdapitypes.ProductSectionList{
		ProductSection: &vcdapitypes.ProductSection{
			Info:     "Custom properties",
			Property: []*vcdapitypes.Property{},
		},
	}
	for key, value := range props {
		property := &vcdapitypes.Property{
			UserConfigurable: true,
			Type:             "string",
			Key:              key,
			Label:            key,
			Value:            &vcdapitypes.Value{Value: value},
		}
		vmProperties.ProductSection.Property = append(vmProperties.ProductSection.Property, property)
	}

	// Set guest properties on the VM
	_, err := vm.SetProductSectionList(vmProperties)
	if err != nil {
		return fmt.Errorf("error setting guest properties for VM: %w", err)
	}
	return nil
}

func addMetadata(vm *govcd.VM, metadata *map[string]string) error {
	// Nothing to do here.
	if metadata == nil {
		return nil
	}

	for key, val := range *metadata {
		err := vm.AddMetadataEntry(vcdapitypes.MetadataStringValue, key, val)
		if err != nil {
			return fmt.Errorf("error adding metadata for VM: %w", err)
		}
	}
	return nil
}

func setComputerName(vm *govcd.VM, machineName string) error {
	customizationSection, err := vm.GetGuestCustomizationSection()
	if err != nil {
		return fmt.Errorf("error retrieving guest customization section for VM: %w", err)
	}

	customizationSection.ComputerName = machineName

	_, err = vm.SetGuestCustomizationSection(customizationSection)
	if err != nil {
		return fmt.Errorf("error adding metadata for VM: %w", err)
	}
	return nil
}
