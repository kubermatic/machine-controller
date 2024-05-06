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

package openstack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/bootfromvolume"
	osextendedstatus "github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/extendedstatus"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/schedulerhints"
	osservers "github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	osfloatingips "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	osnetworks "github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/pagination"
	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	openstacktypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/openstack/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	cloudproviderutil "github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	floatingIPReleaseFinalizer = "kubermatic.io/release-openstack-floating-ip"
	floatingIPIDAnnotationKey  = "kubermatic.io/release-openstack-floating-ip"
	clientTimeout              = 1 * time.Minute
)

// clientGetterFunc returns an OpenStack client.
type clientGetterFunc func(c *Config) (*gophercloud.ProviderClient, error)

// portReadinessWaiterFunc waits for the port with the given ID to be available.
type portReadinessWaiterFunc func(ctx context.Context, instanceLog *zap.SugaredLogger, netClient *gophercloud.ServiceClient, serverID string, networkID string, instanceReadyCheckPeriod time.Duration, instanceReadyCheckTimeout time.Duration) error

type provider struct {
	configVarResolver   *providerconfig.ConfigVarResolver
	clientGetter        clientGetterFunc
	portReadinessWaiter portReadinessWaiterFunc
}

// New returns a openstack provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{
		configVarResolver:   configVarResolver,
		clientGetter:        getClient,
		portReadinessWaiter: waitForPort,
	}
}

type Config struct {
	IdentityEndpoint            string
	ApplicationCredentialID     string
	ApplicationCredentialSecret string
	Username                    string
	Password                    string
	DomainName                  string
	ProjectName                 string
	ProjectID                   string
	TokenID                     string
	Region                      string
	ComputeAPIVersion           string

	// Machine details
	Image                 string
	Flavor                string
	SecurityGroups        []string
	Network               string
	Subnet                string
	FloatingIPPool        string
	AvailabilityZone      string
	TrustDevicePath       bool
	RootDiskSizeGB        *int
	RootDiskVolumeType    string
	NodeVolumeAttachLimit *uint
	ServerGroup           string

	InstanceReadyCheckPeriod  time.Duration
	InstanceReadyCheckTimeout time.Duration

	Tags map[string]string
}

const (
	machineUIDMetaKey = "machine-uid"
	securityGroupName = "kubernetes-v1"
	ovhAuthURL        = "auth.cloud.ovh.net"
)

// Protects floating ip assignment.
var floatingIPAssignLock = &sync.Mutex{}

// Get the Project name from config or env var. If not defined fallback to tenant name.
func (p *provider) getProjectNameOrTenantName(rawConfig *openstacktypes.RawConfig) (string, error) {
	projectName, err := p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ProjectName, "OS_PROJECT_NAME")
	if err == nil && len(projectName) > 0 {
		return projectName, nil
	}

	//fallback to tenantName.
	return p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.TenantName, "OS_TENANT_NAME")
}

// Get the Project id from config or env var. If not defined fallback to tenant id.
func (p *provider) getProjectIDOrTenantID(rawConfig *openstacktypes.RawConfig) (string, error) {
	projectID, err := p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ProjectID, "OS_PROJECT_ID")
	if err == nil && len(projectID) > 0 {
		return projectID, nil
	}

	//fallback to tenantName.
	return p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.TenantID, "OS_TENANT_ID")
}

func (p *provider) getConfigAuth(c *Config, rawConfig *openstacktypes.RawConfig) error {
	var err error
	c.ApplicationCredentialID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ApplicationCredentialID, "OS_APPLICATION_CREDENTIAL_ID")
	if err != nil {
		return fmt.Errorf("failed to get the value of \"applicationCredentialID\" field, error = %w", err)
	}
	if c.ApplicationCredentialID != "" {
		c.ApplicationCredentialSecret, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ApplicationCredentialSecret, "OS_APPLICATION_CREDENTIAL_SECRET")
		if err != nil {
			return fmt.Errorf("failed to get the value of \"applicationCredentialSecret\" field, error = %w", err)
		}
		return nil
	}
	c.Username, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Username, "OS_USER_NAME")
	if err != nil {
		return fmt.Errorf("failed to get the value of \"username\" field, error = %w", err)
	}
	c.Password, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Password, "OS_PASSWORD")
	if err != nil {
		return fmt.Errorf("failed to get the value of \"password\" field, error = %w", err)
	}
	c.ProjectName, err = p.getProjectNameOrTenantName(rawConfig)
	if err != nil {
		return fmt.Errorf("failed to get the value of \"projectName\" field or fallback to \"tenantName\" field, error = %w", err)
	}
	c.ProjectID, err = p.getProjectIDOrTenantID(rawConfig)
	if err != nil {
		return fmt.Errorf("failed to get the value of \"projectID\" or fallback to\"tenantID\" field, error = %w", err)
	}
	return nil
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, *openstacktypes.RawConfig, error) {
	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := openstacktypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg := Config{}
	cfg.IdentityEndpoint, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.IdentityEndpoint, "OS_AUTH_URL")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"identityEndpoint\" field, error = %w", err)
	}

	// Retrieve authentication config, username/password or application credentials.
	err = p.getConfigAuth(&cfg, rawConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to retrieve authentication credentials, error = %w", err)
	}

	// Ignore Region not found as Region might not be found and we can default it later.
	cfg.Region, _ = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Region, "OS_REGION_NAME")

	cfg.InstanceReadyCheckPeriod, err = p.configVarResolver.GetConfigVarDurationValueOrDefault(rawConfig.InstanceReadyCheckPeriod, 5*time.Second)
	if err != nil {
		return nil, nil, nil, fmt.Errorf(`failed to get the value of "InstanceReadyCheckPeriod" field, error = %w`, err)
	}

	cfg.InstanceReadyCheckTimeout, err = p.configVarResolver.GetConfigVarDurationValueOrDefault(rawConfig.InstanceReadyCheckTimeout, 10*time.Second)
	if err != nil {
		return nil, nil, nil, fmt.Errorf(`failed to get the value of "InstanceReadyCheckTimeout" field, error = %w`, err)
	}

	// We ignore errors here because the OS domain is only required when using Identity API V3.
	cfg.DomainName, _ = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.DomainName, "OS_DOMAIN_NAME")
	cfg.TokenID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.TokenID)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.Image, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Image)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.Flavor, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Flavor)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, securityGroup := range rawConfig.SecurityGroups {
		securityGroupValue, err := p.configVarResolver.GetConfigVarStringValue(securityGroup)
		if err != nil {
			return nil, nil, nil, err
		}
		cfg.SecurityGroups = append(cfg.SecurityGroups, securityGroupValue)
	}

	cfg.Network, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Network)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.Subnet, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Subnet)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.FloatingIPPool, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.FloatingIPPool)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.AvailabilityZone, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.AvailabilityZone)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.TrustDevicePath, _, err = p.configVarResolver.GetConfigVarBoolValue(rawConfig.TrustDevicePath)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.ComputeAPIVersion, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ComputeAPIVersion)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.RootDiskSizeGB = rawConfig.RootDiskSizeGB
	cfg.RootDiskVolumeType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.RootDiskVolumeType)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg.NodeVolumeAttachLimit = rawConfig.NodeVolumeAttachLimit
	cfg.Tags = rawConfig.Tags
	if cfg.Tags == nil {
		cfg.Tags = map[string]string{}
	}

	cfg.ServerGroup, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ServerGroup)
	if err != nil {
		return nil, nil, nil, err
	}

	return &cfg, pconfig, rawConfig, err
}

func setProviderSpec(rawConfig openstacktypes.RawConfig, provSpec clusterv1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, err
	}

	rawCloudProviderSpec, err := json.Marshal(rawConfig)
	if err != nil {
		return nil, err
	}

	pconfig.CloudProviderSpec = runtime.RawExtension{Raw: rawCloudProviderSpec}
	rawPconfig, err := json.Marshal(pconfig)
	if err != nil {
		return nil, err
	}

	return &runtime.RawExtension{Raw: rawPconfig}, nil
}

func getClient(c *Config) (*gophercloud.ProviderClient, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: c.IdentityEndpoint,
		Username:         c.Username,
		Password:         c.Password,
		DomainName:       c.DomainName,
		// gophercloud internally store projectName/projectID under tenantName/TenantID. We store it under projectName.
		// to be coherent with KPP code
		TenantName:                  c.ProjectName,
		TenantID:                    c.ProjectID,
		TokenID:                     c.TokenID,
		ApplicationCredentialID:     c.ApplicationCredentialID,
		ApplicationCredentialSecret: c.ApplicationCredentialSecret,
	}

	pc, err := goopenstack.NewClient(c.IdentityEndpoint)
	if err != nil {
		return nil, err
	}
	if pc != nil {
		// use the util's HTTP client to benefit, among other things, from its CA bundle.
		pc.HTTPClient = cloudproviderutil.HTTPClientConfig{
			LogPrefix: "[OpenStack API]",
			Timeout:   clientTimeout,
		}.New()
	}

	err = goopenstack.Authenticate(pc, opts)
	return pc, err
}

func (p *provider) AddDefaults(log *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	c, _, rawConfig, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return spec, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := p.clientGetter(c)
	if err != nil {
		return spec, osErrorToTerminalError(log, err, "failed to get a openstack client")
	}

	if c.Region == "" {
		log.Debug("Trying to default region for machine...")
		regions, err := getRegions(client)
		if err != nil {
			return spec, osErrorToTerminalError(log, err, "failed to get regions")
		}
		if len(regions) == 1 {
			log.Debugw("Defaulted region for machine", "region", regions[0].ID)
			rawConfig.Region.Value = regions[0].ID
		} else {
			return spec, fmt.Errorf("could not default region because got '%v' results", len(regions))
		}
	}

	computeClient, err := getNewComputeV2(client, c)
	if err != nil {
		return spec, osErrorToTerminalError(log, err, "failed to get computeClient")
	}

	if c.AvailabilityZone == "" {
		log.Debug("Trying to default availability zone for machine...")
		availabilityZones, err := getAvailabilityZones(computeClient)
		if err != nil {
			return spec, osErrorToTerminalError(log, err, "failed to get availability zones")
		}
		if len(availabilityZones) == 1 {
			log.Debugw("Defaulted availability zone for machine", "zone", availabilityZones[0].ZoneName)
			rawConfig.AvailabilityZone.Value = availabilityZones[0].ZoneName
		}
	}

	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: c.Region})
	if err != nil {
		return spec, err
	}

	if c.Network == "" {
		log.Debug("Trying to default network for machine...")
		net, err := getDefaultNetwork(netClient)
		if err != nil {
			return spec, osErrorToTerminalError(log, err, "failed to default network")
		}
		if net != nil {
			log.Debugw("Defaulted network for machine ", "network", net.Name)
			// Use the id as the name may not be unique
			rawConfig.Network.Value = net.ID
		}
	}

	if c.Subnet == "" {
		log.Debug("Trying to default subnet for machine...")

		networkID := c.Network
		if rawConfig.Network.Value != "" {
			networkID = rawConfig.Network.Value
		}

		net, err := getNetwork(netClient, networkID)
		if err != nil {
			return spec, osErrorToTerminalError(log, err, fmt.Sprintf("failed to get network for subnet defaulting '%s", networkID))
		}
		subnet, err := getDefaultSubnet(netClient, net)
		if err != nil {
			return spec, osErrorToTerminalError(log, err, "error defaulting subnet")
		}
		if subnet != nil {
			log.Debugw("Defaulted subnet for machine", "subnet", *subnet)
			rawConfig.Subnet.Value = *subnet
		}
	}

	spec.ProviderSpec.Value, err = setProviderSpec(*rawConfig, spec.ProviderSpec)
	if err != nil {
		return spec, osErrorToTerminalError(log, err, "error marshaling providerconfig")
	}
	return spec, nil
}

func (p *provider) Validate(_ context.Context, _ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) error {
	c, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if c.ApplicationCredentialID == "" {
		if c.Username == "" {
			return errors.New("username must be configured")
		}

		if c.Password == "" {
			return errors.New("password must be configured")
		}

		if c.ProjectID == "" && c.ProjectName == "" {
			return errors.New("either projectID / tenantID or projectName / tenantName must be configured")
		}
		if c.DomainName == "" {
			return errors.New("domainName must be configured")
		}
	} else {
		if c.ApplicationCredentialSecret == "" {
			return errors.New("applicationCredentialSecret must be configured in conjunction with applicationCredentialID")
		}
	}

	if c.Image == "" {
		return errors.New("image must be configured")
	}

	if c.Flavor == "" {
		return errors.New("flavor must be configured")
	}

	client, err := p.clientGetter(c)
	if err != nil {
		return fmt.Errorf("failed to get a openstack client: %w", err)
	}

	// Required fields.
	if !strings.Contains(c.IdentityEndpoint, ovhAuthURL) {
		if _, err := getRegion(client, c.Region); err != nil {
			return fmt.Errorf("failed to get region %q: %w", c.Region, err)
		}
	}

	// Get OS Compute Client
	computeClient, err := getNewComputeV2(client, c)
	if err != nil {
		return fmt.Errorf("failed to get compute client: %w", err)
	}

	// Get OS Image Client.
	imageClient, err := goopenstack.NewImageServiceV2(client, gophercloud.EndpointOpts{Region: c.Region})
	if err != nil {
		return fmt.Errorf("failed to get image client: %w", err)
	}

	image, err := getImageByName(imageClient, c)
	if err != nil {
		return fmt.Errorf("failed to get image %q: %w", c.Image, err)
	}
	if c.RootDiskSizeGB != nil {
		if *c.RootDiskSizeGB < image.MinDiskGigabytes {
			return fmt.Errorf("rootDiskSize %d is smaller than minimum disk size for image %q(%d)",
				*c.RootDiskSizeGB, image.Name, image.MinDiskGigabytes)
		}
	}

	if _, err := getFlavor(computeClient, c); err != nil {
		return fmt.Errorf("failed to get flavor %q: %w", c.Flavor, err)
	}

	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: c.Region})
	if err != nil {
		return err
	}

	if _, err := getNetwork(netClient, c.Network); err != nil {
		return fmt.Errorf("failed to get network %q: %w", c.Network, err)
	}

	if _, err := getSubnet(netClient, c.Subnet); err != nil {
		return fmt.Errorf("failed to get subnet %q: %w", c.Subnet, err)
	}

	if c.FloatingIPPool != "" {
		if _, err := getNetwork(netClient, c.FloatingIPPool); err != nil {
			return fmt.Errorf("failed to get floating ip pool %q: %w", c.FloatingIPPool, err)
		}
	}

	if _, err := getAvailabilityZone(computeClient, c); err != nil {
		return fmt.Errorf("failed to get availability zone %q: %w", c.AvailabilityZone, err)
	}
	// Optional fields.
	if len(c.SecurityGroups) != 0 {
		for _, s := range c.SecurityGroups {
			if _, err := getSecurityGroup(client, c.Region, s); err != nil {
				return fmt.Errorf("failed to get security group %q: %w", s, err)
			}
		}
	}

	// validate reserved tags.
	if _, ok := c.Tags[machineUIDMetaKey]; ok {
		return fmt.Errorf("the tag with the given name =%s is reserved, choose a different one", machineUIDMetaKey)
	}

	return nil
}

func (p *provider) Create(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	cfg, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := p.clientGetter(cfg)
	if err != nil {
		return nil, osErrorToTerminalError(log, err, "failed to get a openstack client")
	}

	computeClient, err := getNewComputeV2(client, cfg)
	if err != nil {
		return nil, osErrorToTerminalError(log, err, "failed to get a openstack client")
	}

	flavor, err := getFlavor(computeClient, cfg)
	if err != nil {
		return nil, osErrorToTerminalError(log, err, fmt.Sprintf("failed to get flavor %s", cfg.Flavor))
	}

	// Get OS Image Client.
	imageClient, err := goopenstack.NewImageServiceV2(client, gophercloud.EndpointOpts{Region: cfg.Region})
	if err != nil {
		return nil, osErrorToTerminalError(log, err, "failed to get a image client")
	}

	image, err := getImageByName(imageClient, cfg)
	if err != nil {
		return nil, osErrorToTerminalError(log, err, fmt.Sprintf("failed to get image %s", cfg.Image))
	}

	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: cfg.Region})
	if err != nil {
		return nil, err
	}

	network, err := getNetwork(netClient, cfg.Network)
	if err != nil {
		return nil, osErrorToTerminalError(log, err, fmt.Sprintf("failed to get network %s", cfg.Network))
	}

	securityGroups := cfg.SecurityGroups
	if len(securityGroups) == 0 {
		log.Infow("Creating security group for worker nodes", "group", securityGroupName)
		err = ensureKubernetesSecurityGroupExist(log, client, cfg.Region, securityGroupName)
		if err != nil {
			return nil, fmt.Errorf("Error occurred creating security groups: %w", err)
		}
		securityGroups = append(securityGroups, securityGroupName)
	}

	// we check against reserved tags in Validation method.
	allTags := cfg.Tags
	allTags[machineUIDMetaKey] = string(machine.UID)

	serverOpts := osservers.CreateOpts{
		Name:             machine.Spec.Name,
		FlavorRef:        flavor.ID,
		UserData:         []byte(userdata),
		SecurityGroups:   securityGroups,
		AvailabilityZone: cfg.AvailabilityZone,
		Networks:         []osservers.Network{{UUID: network.ID}},
		Metadata:         allTags,
	}

	var createOpts osservers.CreateOptsBuilder = &serverOpts

	if cfg.ServerGroup != "" {
		createOpts = schedulerhints.CreateOptsExt{
			CreateOptsBuilder: createOpts,
			SchedulerHints: schedulerhints.SchedulerHints{
				Group: cfg.ServerGroup,
			},
		}
	}

	var server serverWithExt
	if cfg.RootDiskSizeGB != nil {
		blockDevices := []bootfromvolume.BlockDevice{
			{
				BootIndex:           0,
				DeleteOnTermination: true,
				DestinationType:     bootfromvolume.DestinationVolume,
				SourceType:          bootfromvolume.SourceImage,
				UUID:                image.ID,
				VolumeSize:          *cfg.RootDiskSizeGB,
				VolumeType:          cfg.RootDiskVolumeType,
			},
		}

		createOpts = bootfromvolume.CreateOptsExt{
			CreateOptsBuilder: createOpts,
			BlockDevice:       blockDevices,
		}

		if err := bootfromvolume.Create(computeClient, createOpts).ExtractInto(&server); err != nil {
			return nil, osErrorToTerminalError(log, err, "failed to create server with volume")
		}
	} else {
		// Image ID should only be set in server options when block device
		// mapping is not used. Otherwish an error may occur with some
		// OpenStack providers/versions .e.g. OpenTelekom Cloud.
		serverOpts.ImageRef = image.ID

		if err := osservers.Create(computeClient, createOpts).ExtractInto(&server); err != nil {
			return nil, osErrorToTerminalError(log, err, "failed to create server")
		}
	}

	if cfg.FloatingIPPool != "" {
		instanceLog := log.With("instance", server.ID)

		if err := p.portReadinessWaiter(ctx, instanceLog, netClient, server.ID, network.ID, cfg.InstanceReadyCheckPeriod, cfg.InstanceReadyCheckTimeout); err != nil {
			instanceLog.Infow("Port for instance did not became active", zap.Error(err))
		}

		// Find a free FloatingIP or allocate a new one.
		if err := assignFloatingIPToInstance(instanceLog, data.Update, machine, netClient, server.ID, cfg.FloatingIPPool, cfg.Region, network); err != nil {
			defer deleteInstanceDueToFatalLogged(instanceLog, computeClient, server.ID)
			return nil, fmt.Errorf("failed to assign a floating ip to instance %s: %w", server.ID, err)
		}
	}

	return &osInstance{server: &server}, nil
}

func waitForPort(ctx context.Context, instanceLog *zap.SugaredLogger, netClient *gophercloud.ServiceClient, serverID string, networkID string, checkPeriod time.Duration, checkTimeout time.Duration) error {
	started := time.Now()
	instanceLog.Info("Waiting for the port to become active...")

	portIsReady := func(context.Context) (bool, error) {
		port, err := getInstancePort(netClient, serverID, networkID)
		if err != nil {
			tErr := osErrorToTerminalError(instanceLog, err, fmt.Sprintf("failed to get current instance port %s", serverID))
			if isTerminalErr, _, _ := cloudprovidererrors.IsTerminalError(tErr); isTerminalErr {
				return true, tErr
			}
			// Only log the error but don't exit. in case of a network failure we want to retry.
			instanceLog.Errorw("Failed to get current instance port", zap.Error(err))
			return false, nil
		}

		return port.Status == "ACTIVE", nil
	}

	if err := wait.PollUntilContextTimeout(ctx, checkPeriod, checkTimeout, false, portIsReady); err != nil {
		if wait.Interrupted(err) {
			// In case we have a timeout, include the timeout details
			return fmt.Errorf("instance port became not active after %f seconds", checkTimeout.Seconds())
		}
		// Some terminal error happened
		return fmt.Errorf("failed to wait for instance port to become active: %w", err)
	}

	instanceLog.Infow("Instance port became active", "elapsed", time.Since(started).Round(time.Second))
	return nil
}

func deleteInstanceDueToFatalLogged(instanceLog *zap.SugaredLogger, computeClient *gophercloud.ServiceClient, serverID string) {
	instanceLog.Info("Deleting instance due to fatal error during machine creation...")
	if err := osservers.Delete(computeClient, serverID).ExtractErr(); err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to delete the instance %s. Please take care of manually deleting the instance: %w", serverID, err))
		return
	}
	instanceLog.Info("Instance got deleted")
}

func (p *provider) Cleanup(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	var hasFloatingIPReleaseFinalizer bool
	if finalizers := sets.NewString(machine.Finalizers...); finalizers.Has(floatingIPReleaseFinalizer) {
		hasFloatingIPReleaseFinalizer = true
	}

	instance, err := p.Get(ctx, log, machine, data)
	if err != nil {
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			if hasFloatingIPReleaseFinalizer {
				if err := p.cleanupFloatingIP(log, machine, data.Update); err != nil {
					return false, fmt.Errorf("failed to clean up floating ip: %w", err)
				}
			}
			return true, nil
		}
		return false, err
	}

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := p.clientGetter(c)
	if err != nil {
		return false, osErrorToTerminalError(log, err, "failed to get a openstack client")
	}

	computeClient, err := getNewComputeV2(client, c)
	if err != nil {
		return false, osErrorToTerminalError(log, err, "failed to get compute client")
	}

	if err := osservers.Delete(computeClient, instance.ID()).ExtractErr(); err != nil && !errors.As(err, &gophercloud.ErrDefault404{}) {
		return false, osErrorToTerminalError(log, err, "failed to delete instance")
	}

	if hasFloatingIPReleaseFinalizer {
		return false, p.cleanupFloatingIP(log, machine, data.Update)
	}

	return false, nil
}

func (p *provider) Get(_ context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := p.clientGetter(c)
	if err != nil {
		return nil, osErrorToTerminalError(log, err, "failed to get a openstack client")
	}

	computeClient, err := getNewComputeV2(client, c)
	if err != nil {
		return nil, osErrorToTerminalError(log, err, "failed to get compute client")
	}

	var allServers []serverWithExt
	pager := osservers.List(computeClient, osservers.ListOpts{Name: machine.Spec.Name})
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		var servers []serverWithExt
		err = osservers.ExtractServersInto(page, &servers)
		if err != nil {
			return false, osErrorToTerminalError(log, err, "failed to extract instance info")
		}
		allServers = append(allServers, servers...)
		return true, nil
	})
	if err != nil {
		return nil, osErrorToTerminalError(log, err, "failed to list instances")
	}

	for i, s := range allServers {
		if s.Metadata[machineUIDMetaKey] == string(machine.UID) {
			return &osInstance{server: &allServers[i]}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) MigrateUID(_ context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, newUID types.UID) error {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := p.clientGetter(c)
	if err != nil {
		return osErrorToTerminalError(log, err, "failed to get a openstack client")
	}

	computeClient, err := getNewComputeV2(client, c)
	if err != nil {
		return osErrorToTerminalError(log, err, "failed to get compute client")
	}

	var allServers []serverWithExt
	pager := osservers.List(computeClient, osservers.ListOpts{Name: machine.Spec.Name})
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		var servers []serverWithExt
		err = osservers.ExtractServersInto(page, &servers)
		if err != nil {
			return false, osErrorToTerminalError(log, err, "failed to extract instance info")
		}
		allServers = append(allServers, servers...)
		return true, nil
	})
	if err != nil {
		return osErrorToTerminalError(log, err, "failed to list instances")
	}

	for _, s := range allServers {
		if s.Metadata[machineUIDMetaKey] == string(machine.UID) {
			metadataOpts := osservers.MetadataOpts(s.Metadata)
			metadataOpts[machineUIDMetaKey] = string(newUID)
			response := osservers.UpdateMetadata(computeClient, s.ID, metadataOpts)
			if response.Err != nil {
				return fmt.Errorf("failed to update instance metadata with new UID: %w", err)
			}
		}
	}

	return nil
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	c, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse config: %w", err)
	}

	cc := &openstacktypes.CloudConfig{
		Global: openstacktypes.GlobalOpts{
			AuthURL:                     c.IdentityEndpoint,
			Username:                    c.Username,
			Password:                    c.Password,
			DomainName:                  c.DomainName,
			ProjectName:                 c.ProjectName,
			ProjectID:                   c.ProjectID,
			Region:                      c.Region,
			ApplicationCredentialSecret: c.ApplicationCredentialSecret,
			ApplicationCredentialID:     c.ApplicationCredentialID,
		},
		LoadBalancer: openstacktypes.LoadBalancerOpts{
			ManageSecurityGroups: true,
		},
		BlockStorage: openstacktypes.BlockStorageOpts{
			BSVersion:       "auto",
			TrustDevicePath: c.TrustDevicePath,
			IgnoreVolumeAZ:  true,
		},
		Version: spec.Versions.Kubelet,
	}
	if c.NodeVolumeAttachLimit != nil {
		cc.BlockStorage.NodeVolumeAttachLimit = *c.NodeVolumeAttachLimit
	}

	s, err := openstacktypes.CloudConfigToString(cc)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert the cloud-config to string: %w", err)
	}
	return s, "openstack", nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["size"] = c.Flavor
		labels["image"] = c.Image
		labels["region"] = c.Region
	}

	return labels, err
}

type serverWithExt struct {
	osservers.Server
	osextendedstatus.ServerExtendedStatusExt
}

type osInstance struct {
	server *serverWithExt
}

func (d *osInstance) Name() string {
	return d.server.Name
}

func (d *osInstance) ID() string {
	return d.server.ID
}

func (d *osInstance) ProviderID() string {
	if d.server == nil || d.server.ID == "" {
		return ""
	}
	return "openstack:///" + d.server.ID
}

func (d *osInstance) Addresses() map[string]corev1.NodeAddressType {
	addresses := map[string]corev1.NodeAddressType{}
	for _, networkAddresses := range d.server.Addresses {
		for _, element := range networkAddresses.([]interface{}) {
			address := element.(map[string]interface{})
			addresses[address["addr"].(string)] = ""
		}
	}

	return addresses
}

func (d *osInstance) Status() instance.Status {
	switch d.server.Status {
	case "IN_PROGRESS":
		return instance.StatusCreating
	case "ACTIVE":
		return instance.StatusRunning
	default:
		return instance.StatusUnknown
	}
}

// osErrorToTerminalError judges if the given error
// can be qualified as a "terminal" error, for more info see v1alpha1.MachineStatus
//
// if the given error doesn't qualify the error passed as an argument will be returned.
func osErrorToTerminalError(log *zap.SugaredLogger, err error, msg string) error {
	var errUnauthorized gophercloud.ErrDefault401
	if errors.As(err, &errUnauthorized) {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("A request has been rejected due to invalid credentials which were taken from the MachineSpec: %v", errUnauthorized),
		}
	}

	var errForbidden gophercloud.ErrDefault403
	if errors.As(err, &errForbidden) {
		terr := cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("%s. The request against the OpenStack API is forbidden: %s", msg, errForbidden.Error()),
		}

		// The response from OpenStack might contain a more detailed message
		info := &forbiddenResponse{}
		if err := json.Unmarshal(errForbidden.Body, info); err != nil {
			// We just log here as we just do this to make the response more pretty
			log.Errorw("Failed to unmarshal response body from 403 response from OpenStack API", "body", errForbidden.Body, zap.Error(err))
			return terr
		}

		// If we have more details, interpret them
		if info.Forbidden.Message != "" {
			terr.Message = fmt.Sprintf("%s. The request against the OpenStack API is forbidden: %s", msg, info.Forbidden.Message)
			if strings.Contains(info.Forbidden.Message, "Quota exceeded") {
				terr.Reason = common.InsufficientResourcesMachineError
			}
		}

		return terr
	}

	return fmt.Errorf("%s: %w", msg, err)
}

// forbiddenResponse is a potential response body from the OpenStack API when the request is forbidden (code: 403).
type forbiddenResponse struct {
	Forbidden struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"forbidden"`
}

func (p *provider) cleanupFloatingIP(log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, updater cloudprovidertypes.MachineUpdater) error {
	floatingIPID, exists := machine.Annotations[floatingIPIDAnnotationKey]
	if !exists {
		return osErrorToTerminalError(log, fmt.Errorf("failed to release floating ip"),
			fmt.Sprintf("%s finalizer exists but %s annotation does not", floatingIPReleaseFinalizer, floatingIPIDAnnotationKey))
	}

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := p.clientGetter(c)
	if err != nil {
		return osErrorToTerminalError(log, err, "failed to get a openstack client")
	}
	netClient, err := goopenstack.NewNetworkV2(client, gophercloud.EndpointOpts{Region: c.Region})
	if err != nil {
		return fmt.Errorf("failed to create the networkv2 client for region %s: %w", c.Region, err)
	}
	if err := osfloatingips.Delete(netClient, floatingIPID).ExtractErr(); err != nil && !errors.As(err, &gophercloud.ErrDefault404{}) {
		return fmt.Errorf("failed to delete floating ip %s: %w", floatingIPID, err)
	}
	if err := updater(machine, func(m *clusterv1alpha1.Machine) {
		finalizers := sets.NewString(m.Finalizers...)
		finalizers.Delete(floatingIPReleaseFinalizer)
		m.Finalizers = finalizers.List()
	}); err != nil {
		return fmt.Errorf("failed to delete %s finalizer from Machine: %w", floatingIPReleaseFinalizer, err)
	}

	return nil
}

func assignFloatingIPToInstance(instanceLog *zap.SugaredLogger, machineUpdater cloudprovidertypes.MachineUpdater, machine *clusterv1alpha1.Machine, netClient *gophercloud.ServiceClient, instanceID, floatingIPPoolName, region string, network *osnetworks.Network) error {
	port, err := getInstancePort(netClient, instanceID, network.ID)
	if err != nil {
		return fmt.Errorf("failed to get instance port for network %s in region %s: %w", network.ID, region, err)
	}

	floatingIPPool, err := getNetwork(netClient, floatingIPPoolName)
	if err != nil {
		return osErrorToTerminalError(instanceLog, err, fmt.Sprintf("failed to get floating IP pool %q", floatingIPPoolName))
	}

	instanceLog.Info("Assigning a floating IP to instance")

	floatingIPAssignLock.Lock()
	defer floatingIPAssignLock.Unlock()

	freeFloatingIps, err := getFreeFloatingIPs(netClient, floatingIPPool)
	if err != nil {
		return osErrorToTerminalError(instanceLog, err, "failed to get free floating ips")
	}

	var ip *osfloatingips.FloatingIP
	if len(freeFloatingIps) < 1 {
		if ip, err = createFloatingIP(netClient, port.ID, floatingIPPool); err != nil {
			return osErrorToTerminalError(instanceLog, err, "failed to allocate a floating ip")
		}
		if err := machineUpdater(machine, func(m *clusterv1alpha1.Machine) {
			m.Finalizers = append(m.Finalizers, floatingIPReleaseFinalizer)
			if m.Annotations == nil {
				m.Annotations = map[string]string{}
			}
			m.Annotations[floatingIPIDAnnotationKey] = ip.ID
		}); err != nil {
			return fmt.Errorf("failed to add floating ip release finalizer after allocating floating ip: %w", err)
		}
	} else {
		freeIP := freeFloatingIps[0]
		ip, err = osfloatingips.Update(netClient, freeIP.ID, osfloatingips.UpdateOpts{
			PortID: &port.ID,
		}).Extract()
		if err != nil {
			return fmt.Errorf("failed to update floating IP %s(%s): %w", freeIP.ID, freeIP.FloatingIP, err)
		}

		// We're now going to wait 3 seconds and check if the IP is still ours. If not, we're going to fail
		// On our reference system it took ~3 seconds for a full FloatingIP allocation (Including creating a new one). It took ~600ms just for assigning one.
		time.Sleep(floatingReassignIPCheckPeriod)
		currentIP, err := osfloatingips.Get(netClient, ip.ID).Extract()
		if err != nil {
			return fmt.Errorf("failed to load floating IP %s after assignment has been done: %w", ip.FloatingIP, err)
		}
		// Verify if the port is still the one we set it to
		if currentIP.PortID != port.ID {
			return fmt.Errorf("floatingIP %s got reassigned", currentIP.FloatingIP)
		}
	}

	instanceLog.Infow("Successfully assigned the floating IP to instance", "ip", ip.FloatingIP)
	return nil
}

func (p *provider) SetMetricsForMachines(_ clusterv1alpha1.MachineList) error {
	return nil
}
