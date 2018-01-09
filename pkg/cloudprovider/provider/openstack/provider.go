package openstack

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	osextendedstatus "github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/extendedstatus"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	osservers "github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/pagination"
	cloudproviererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

type provider struct{}

func New() *provider {
	return &provider{}
}

type config struct {
	// Auth details
	IdentityEndpoint string `json:"identityEndpoint"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	DomainName       string `json:"domainName"`
	TenantName       string `json:"tenantName"`
	TokenID          string `json:"tokenId"`

	// Machine details
	Image            string   `json:"image"`
	Flavor           string   `json:"flavor"`
	SecurityGroups   []string `json:"securityGroups"`
	Network          string   `json:"network"`
	Subnet           string   `json:"subnet"`
	FloatingIPPool   string   `json:"floatingIpPool"`
	AvailabilityZone string   `json:"availabilityZone"`
	Region           string   `json:"region"`
}

const (
	machineUIDMetaKey = "machine-uid"
	securityGroupName = "kubernetes-v1"
)

func getConfig(s runtime.RawExtension) (*config, *providerconfig.Config, error) {
	pconfig := providerconfig.Config{}
	err := json.Unmarshal(s.Raw, &pconfig)
	if err != nil {
		return nil, nil, err
	}
	c := config{}
	err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &c)
	return &c, &pconfig, err
}

func getClient(c *config) (*gophercloud.ProviderClient, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: c.IdentityEndpoint,
		Username:         c.Username,
		Password:         c.Password,
		DomainName:       c.DomainName,
		TenantName:       c.TenantName,
		TokenID:          c.TokenID,
	}

	return openstack.AuthenticatedClient(opts)
}

func (p *provider) Validate(spec v1alpha1.MachineSpec) error {
	c, _, err := getConfig(spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	client, err := getClient(c)
	if err != nil {
		return fmt.Errorf("failed to get a openstack client: %v", err)
	}

	// Required fields
	if _, err := getRegion(client, c.Region); err != nil {
		return fmt.Errorf("failed to get region %q: %v", c.Region, err)
	}

	if _, err := getImageByName(client, c.Region, c.Image); err != nil {
		return fmt.Errorf("failed to get image %q: %v", c.Image, err)
	}

	if _, err := getFlavor(client, c.Region, c.Flavor); err != nil {
		return fmt.Errorf("failed to get flavor %q: %v", c.Flavor, err)
	}

	if _, err := getNetwork(client, c.Region, c.Network); err != nil {
		return fmt.Errorf("failed to get network %q: %v", c.Network, err)
	}

	if _, err := getSubnet(client, c.Region, c.Subnet); err != nil {
		return fmt.Errorf("failed to get subnet %q: %v", c.Subnet, err)
	}

	if _, err := getNetwork(client, c.Region, c.FloatingIPPool); err != nil {
		return fmt.Errorf("failed to get floating ip pool %q: %v", c.FloatingIPPool, err)
	}

	if _, err := getAvailabilityZone(client, c.Region, c.AvailabilityZone); err != nil {
		return fmt.Errorf("failed to get availability zone %q: %v", c.AvailabilityZone, err)
	}

	// Optional fields
	if len(c.SecurityGroups) != 0 {
		for _, s := range c.SecurityGroups {
			if _, err := getSecurityGroup(client, c.Region, s); err != nil {
				return fmt.Errorf("failed to get security group %q: %v", s, err)
			}
		}
	}

	return nil
}

func (p *provider) Create(machine *v1alpha1.Machine, userdata string, publicKey rsa.PublicKey) (instance.Instance, error) {
	c, _, err := getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	client, err := getClient(c)
	if err != nil {
		return nil, fmt.Errorf("failed to get a openstack client: %v", err)
	}

	name, err := ensureSSHKeysExist(client, c.Region, "machine-controller", publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed ensure that the ssh key exists: %v", err)
	}

	flavor, err := getFlavor(client, c.Region, c.Flavor)
	if err != nil {
		return nil, fmt.Errorf("failed to get flavor %s: %v", c.Flavor, err)
	}

	image, err := getImageByName(client, c.Region, c.Image)
	if err != nil {
		return nil, fmt.Errorf("failed to get image %s: %v", c.Image, err)
	}

	network, err := getNetwork(client, c.Region, c.Network)
	if err != nil {
		return nil, fmt.Errorf("failed to get network %s: %v", c.Network, err)
	}

	err = ensureKubernetesSecurityGroupExist(client, c.Region, securityGroupName)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure that the kubernetes security group %q exists: %v", securityGroupName, err)
	}

	serverOpts := osservers.CreateOpts{
		Name:             machine.Spec.Name,
		FlavorRef:        flavor.ID,
		ImageRef:         image.ID,
		UserData:         []byte(userdata),
		SecurityGroups:   append(c.SecurityGroups, securityGroupName),
		AvailabilityZone: c.AvailabilityZone,
		Networks:         []osservers.Network{{UUID: network.ID}},
		Metadata: map[string]string{
			machineUIDMetaKey: string(machine.UID),
		},
	}
	computeClient, err := goopenstack.NewComputeV2(client, gophercloud.EndpointOpts{Availability: gophercloud.AvailabilityPublic, Region: c.Region})
	if err != nil {
		return nil, err
	}

	var server ServerWithExt
	err = osservers.Create(computeClient, keypairs.CreateOptsExt{
		serverOpts,
		name,
	}).ExtractInto(&server)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %v", err)
	}
	return &osInstance{server: &server}, nil
}

func (p *provider) Delete(machine *v1alpha1.Machine) error {
	c, _, err := getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	client, err := getClient(c)
	if err != nil {
		return fmt.Errorf("failed to get a openstack client: %v", err)
	}

	computeClient, err := goopenstack.NewComputeV2(client, gophercloud.EndpointOpts{Availability: gophercloud.AvailabilityPublic, Region: c.Region})
	if err != nil {
		return err
	}

	s, err := p.Get(machine)
	if err != nil {
		if err == cloudproviererrors.InstanceNotFoundErr {
			return nil
		}
		return err
	}

	return osservers.Delete(computeClient, s.ID()).ExtractErr()
}

func (p *provider) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	c, _, err := getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	client, err := getClient(c)
	if err != nil {
		return nil, fmt.Errorf("failed to get a openstack client: %v", err)
	}

	computeClient, err := goopenstack.NewComputeV2(client, gophercloud.EndpointOpts{Availability: gophercloud.AvailabilityPublic, Region: c.Region})
	if err != nil {
		return nil, err
	}

	var allServers []ServerWithExt
	pager := osservers.List(computeClient, osservers.ListOpts{Name: machine.Spec.Name})
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		var servers []ServerWithExt
		err = osservers.ExtractServersInto(page, &servers)
		if err != nil {
			return false, err
		}
		allServers = append(allServers, servers...)
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	for _, s := range allServers {
		if s.Metadata[machineUIDMetaKey] == string(machine.UID) {
			return &osInstance{server: &s}, nil
		}
	}

	return nil, cloudproviererrors.InstanceNotFoundErr
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	c, _, err := getConfig(spec.ProviderConfig)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse config: %v", err)
	}

	config = fmt.Sprintf(`
[Global]
auth-url = "%s"
username = "%s"
password = "%s"
domain-name="%s"
tenant-name = "%s"
`, c.IdentityEndpoint, c.Username, c.Password, c.DomainName, c.TenantName)
	return config, "openstack", nil
}

type ServerWithExt struct {
	osservers.Server
	osextendedstatus.ServerExtendedStatusExt
}

type osInstance struct {
	server *ServerWithExt
}

func (d *osInstance) Name() string {
	return d.server.Name
}

func (d *osInstance) ID() string {
	return d.server.ID
}

func (d *osInstance) Status() instance.State {
	return instance.InstanceRunning
}

func (d *osInstance) Addresses() []string {
	return []string{d.server.AccessIPv4, d.server.AccessIPv6}
}
