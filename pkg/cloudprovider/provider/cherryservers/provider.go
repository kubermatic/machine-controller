package cherryservers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/cherryservers/cherrygo"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"golang.org/x/crypto/ssh"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const privateRSAKeyBitSize = 4096

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

type RawConfig struct {
	TeamID    providerconfig.ConfigVarString `json:"teamID"`
	ProjectID providerconfig.ConfigVarString `json:"projectID"`
	Token     providerconfig.ConfigVarString `json:"token,omitempty"`
	Plan      providerconfig.ConfigVarString `json:"plan"`
	Location  providerconfig.ConfigVarString `json:"location"`
	Labels    map[string]string              `json:"labels,omitempty"`
}

type Config struct {
	TeamID    string
	ProjectID string
	Token     string
	Plan      string
	Location  string
	Labels    map[string]string
}

func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) getConfig(s v1alpha1.ProviderSpec) (*Config, *providerconfig.Config, error) {
	if s.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	pconfig := providerconfig.Config{}
	err := json.Unmarshal(s.Value.Raw, &pconfig)
	if err != nil {
		return nil, nil, err
	}

	rawConfig := RawConfig{}
	if err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
		return nil, nil, err
	}

	c := Config{}
	c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, "CS_TOKEN")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"token\" field, error = %v", err)
	}
	c.Plan, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Plan)
	if err != nil {
		return nil, nil, err
	}

	c.ProjectID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ProjectID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"projectID\" field, error = %v", err)
	}

	c.TeamID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.TeamID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"teamID\" field, error = %v", err)
	}

	c.Location, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Location)
	if err != nil {
		return nil, nil, err
	}
	c.Labels = rawConfig.Labels
	return &c, &pconfig, err
}

func getNameForOS(os providerconfig.OperatingSystem) (string, error) {
	switch os {
	case providerconfig.OperatingSystemUbuntu:
		return "Ubuntu 18.04 64bit", nil
	case providerconfig.OperatingSystemCentOS:
		return "CentOS 7 64bit", nil
	}
	return "", providerconfig.ErrOSNotSupported
}

func getClient(token string) *cherrygo.Client {
	os.Setenv("CHERRY_AUTH_TOKEN", token)
	client, err := cherrygo.NewClient()
	if err != nil {
		return nil
	}
	return client
}

func (p *provider) Validate(spec v1alpha1.MachineSpec) error {
	c, pc, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if c.Token == "" {
		return errors.New("token is missing")
	}

	_, err = getNameForOS(pc.OperatingSystem)
	if err != nil {
		return fmt.Errorf("invalid/not supported operating system specified %q: %v", pc.OperatingSystem, err)
	}

	return nil
}

func (p *provider) Create(machine *v1alpha1.Machine, _ *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)

	osName, err := getNameForOS(pc.OperatingSystem)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Invalid operating system specified %q, details = %v", pc.OperatingSystem, err),
		}
	}

	teamID, err := strconv.Atoi(c.TeamID)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Invalid TeamID, details = %v", err),
		}
	}

	var planID int = 0
	plans, _, err := client.Plans.List(teamID)
	for _, plan := range plans {
		if plan.Name == c.Plan {
			planID = plan.ID
			break
		}
	}

	serverCreateRequest := cherrygo.CreateServer{
		ProjectID:   c.ProjectID,
		Hostname:    machine.Name,
		Image:       osName,
		Region:      c.Location,
		SSHKeys:     []string{},
		IPAddresses: []string{},
		PlanID:      strconv.Itoa(planID),
		UserData:    base64.StdEncoding.EncodeToString([]byte(userdata)),
	}

	server, _, err := client.Server.Create(c.ProjectID, &serverCreateRequest)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to create server = %v", err),
		}
	}

	return &cherryServer{server: server}, nil
}

type cherryServer struct {
	server cherrygo.Server
}

func (s *cherryServer) Name() string {
	return s.server.Hostname
}

func (s *cherryServer) ID() string {
	return strconv.Itoa(s.server.ID)
}

func (s *cherryServer) Addresses() []string {
	var addresses []string
	for _, ip := range s.server.IPAddresses {
		addresses = append(addresses, ip.Address)
	}
	return addresses
}

func (s *cherryServer) Status() instance.Status {
	if s.server.State != "active" {
		return instance.StatusCreating
	}
	return instance.StatusRunning
}

func (p *provider) Get(machine *v1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)

	servers, _, err := client.Servers.List(c.ProjectID)
	if err != nil {
		return nil, err
	}

	for _, server := range servers {
		if server.Hostname == machine.Name {
			srv, _, err := client.Server.List(strconv.Itoa(server.ID))
			if err != nil {
				return nil, err
			}
			return &cherryServer{server: srv}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	return nil
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) SetMetricsForMachines(machines v1alpha1.MachineList) error {
	return nil
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["plan"] = c.Plan
		labels["location"] = c.Location
	}

	return labels, err
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	server, err := p.Get(machine, data)
	if err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
			return true, nil
		}
		return false, err
	}

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)

	_, _, err = client.Server.Delete(
		&cherrygo.DeleteServer{
			ID: server.ID(),
		})
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.DeleteMachineError,
			Message: fmt.Sprintf("Could not delete machine, due to %v", err),
		}
	}

	return false, nil
}

func NewKey() (privateKey []byte, publicKey string, err error) {
	tmpRSAKeyPair, err := rsa.GenerateKey(rand.Reader, privateRSAKeyBitSize)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create private RSA key: %v", err)
	}

	if err := tmpRSAKeyPair.Validate(); err != nil {
		return nil, "", fmt.Errorf("failed to validate private RSA key: %v", err)
	}

	pubKey, err := ssh.NewPublicKey(&tmpRSAKeyPair.PublicKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate ssh public key: %v", err)
	}

	privateDer := x509.MarshalPKCS1PrivateKey(tmpRSAKeyPair)
	privateBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privateDer,
	}
	privatePEM := pem.EncodeToMemory(&privateBlock)

	return privatePEM, string(ssh.MarshalAuthorizedKey(pubKey)), nil
}
