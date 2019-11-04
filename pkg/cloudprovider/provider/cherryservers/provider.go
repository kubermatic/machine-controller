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

package cherryservers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/cherryservers/cherrygo"
	"github.com/golang/glog"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	machineUIDTagKey = "machine-uid"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type RawConfig struct {
	TeamID    providerconfig.ConfigVarString `json:"teamID"`
	ProjectID providerconfig.ConfigVarString `json:"projectID"`
	Token     providerconfig.ConfigVarString `json:"token,omitempty"`
	Plan      providerconfig.ConfigVarString `json:"plan"`
	Location  providerconfig.ConfigVarString `json:"location"`
	Tags      map[string]string              `json:"tags,omitempty"`
}

type Config struct {
	TeamID    string
	ProjectID string
	Token     string
	Plan      string
	Location  string
	Tags      map[string]string
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
	c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, "CHERRYSERVERS_TOKEN")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"token\" field, error = %v", err)
	}
	c.Plan, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Plan)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"plan\" field, error = %v", err)
	}

	c.ProjectID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ProjectID, "CHERRYSERVERS_PROJECT_ID")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"projectID\" field, error = %v", err)
	}

	c.TeamID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.TeamID, "CHERRYSERVERS_TEAM_ID")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"teamID\" field, error = %v", err)
	}

	c.Location, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Location)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"location\" field, error = %v", err)
	}
	c.Tags = rawConfig.Tags
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
	httpClient := &http.Client{}
	return cherrygo.NewClientWithAuthVar(httpClient, token)
}

func getTeamByID(client *cherrygo.Client, ID string) (*cherrygo.Teams, error) {
	teams, _, err := client.Teams.List()
	if err != nil {
		return nil, err
	}

	for _, team := range teams {
		if strconv.Itoa(team.ID) == ID {
			return &team, nil
		}
	}
	return nil, nil
}

func getPlanByName(client *cherrygo.Client, planName string, teamID string) (*cherrygo.Plans, error) {
	tid, err := strconv.Atoi(teamID)
	if err != nil {
		return nil, err
	}

	plans, _, err := client.Plans.List(tid)
	if err != nil {
		return nil, err
	}

	for _, plan := range plans {
		if plan.Name == planName {
			return &plan, nil
		}
	}
	return nil, nil
}

func getServerByTag(client *cherrygo.Client, tag string, projectID string) (*cherrygo.Server, error) {
	servers, _, err := client.Servers.List(projectID)
	if err != nil {
		return nil, err
	}

	for _, server := range servers {
		if server.Tags[machineUIDTagKey] == tag {
			srv, _, err := client.Server.List(strconv.Itoa(server.ID))
			if err != nil {
				return nil, err
			}
			return &srv, nil
		}
	}

	return nil, nil
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

	client := getClient(c.Token)

	if c.TeamID != "" {
		if team, err := getTeamByID(client, c.TeamID); err != nil || team == nil {
			return fmt.Errorf("failed to get teamID")
		}
	}

	if c.Plan != "" {
		if plan, err := getPlanByName(client, c.Plan, c.TeamID); err != nil || plan == nil {
			return fmt.Errorf("failed to get plan")
		}
	}

	if c.Location != "" {
		if c.Location != "EU-East-1" && c.Location != "EU-West-1" {
			return fmt.Errorf("failed to get location")
		}
	}

	if c.ProjectID != "" {
		if _, _, err := client.Project.List(c.ProjectID); err != nil {
			return fmt.Errorf("failed to get projectID")
		}
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

	var planID int
	plans, _, err := client.Plans.List(teamID)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to list servers = %v", err),
		}
	}
	for _, plan := range plans {
		if plan.Name == c.Plan {
			planID = plan.ID
			break
		}
	}

	if c.Tags == nil {
		c.Tags = map[string]string{}
	}
	c.Tags[machineUIDTagKey] = string(machine.UID)

	serverCreateRequest := cherrygo.CreateServer{
		ProjectID:   c.ProjectID,
		Hostname:    machine.Name,
		Image:       osName,
		Region:      c.Location,
		PlanID:      strconv.Itoa(planID),
		UserData:    base64.StdEncoding.EncodeToString([]byte(userdata)),
		Tags:        c.Tags,
		SSHKeys:     []string{},
		IPAddresses: []string{},
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
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to list servers, due to %v", err),
		}
	}

	for _, server := range servers {
		if server.Tags[machineUIDTagKey] == string(machine.UID) {
			srv, _, err := client.Server.List(strconv.Itoa(server.ID))
			if err != nil {
				return nil, cloudprovidererrors.TerminalError{
					Reason:  common.InvalidConfigurationMachineError,
					Message: fmt.Sprintf("Failed to fetch server, due to %v", err),
				}
			}
			return &cherryServer{server: srv}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}
	client := getClient(c.Token)

	server, err := getServerByTag(client, string(machine.UID), c.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get server: %v", err)
	}
	if server == nil {
		glog.Infof("No instance exists for machine %s", machine.Name)
		return nil
	}

	glog.Infof("Setting UID tag for machine %s", machine.Name)
	tags := server.Tags
	tags[machineUIDTagKey] = string(new)
	updateServerRequest := cherrygo.UpdateServer{
		Tags: tags,
	}

	_, response, err := client.Server.Update(strconv.Itoa(server.ID), &updateServerRequest)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to update UID tag, due to %v", err),
		}
	}
	if response.Response.StatusCode != http.StatusOK {
		return fmt.Errorf("got unexpected response code %v, expected %v", response.Response.Status, http.StatusOK)
	}
	glog.Infof("Successfully set UID tag for machine %s", machine.Name)

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
