package digitalocean

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/digitalocean/godo"
	"github.com/golang/glog"
	"golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"

	"k8s.io/apimachinery/pkg/runtime"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

type digitalocean struct{}

func New() *digitalocean {
	return &digitalocean{}
}

type config struct {
	Token             string   `json:"token"`
	Region            string   `json:"region"`
	Size              string   `json:"size"`
	SSHKeys           []sshKey `json:"ssh_keys"`
	Backups           bool     `json:"backups"`
	IPv6              bool     `json:"ipv6"`
	PrivateNetworking bool     `json:"private_networking"`
	Tags              []string `json:"tags"`
}

type sshKey struct {
	//Either specify ID or Fingerprint to use an existing key
	ID          int    `json:"id"`
	Fingerprint string `json:"fingerprint"`
	// Or specify Name & PublicKey so the key will be created
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

const (
	image = "coreos-stable"
)

type TokenSource struct {
	AccessToken string
}

func (t *TokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}
	return token, nil
}

func getClient(token string) *godo.Client {
	tokenSource := &TokenSource{
		AccessToken: token,
	}

	oauthClient := oauth2.NewClient(context.Background(), tokenSource)
	return godo.NewClient(oauthClient)
}

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

func (do *digitalocean) Validate(spec v1alpha1.MachineSpec) error {
	c, _, err := getConfig(spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if c.Token == "" {
		return errors.New("token is missing")
	}

	if c.Region == "" {
		return errors.New("region is missing")
	}

	if c.Size == "" {
		return errors.New("size is missing")
	}

	ctx := context.TODO()
	client := getClient(c.Token)

	regions, _, err := client.Regions.List(ctx, &godo.ListOptions{PerPage: 1000})
	if err != nil {
		return err
	}
	var foundRegion bool
	for _, region := range regions {
		if region.Slug == c.Region {
			foundRegion = true
			break
		}
	}
	if !foundRegion {
		return fmt.Errorf("region %q not found", c.Region)
	}

	sizes, _, err := client.Sizes.List(ctx, &godo.ListOptions{PerPage: 1000})
	if err != nil {
		return err
	}
	var foundSize bool
	for _, size := range sizes {
		if size.Slug == c.Size {
			if !size.Available {
				return fmt.Errorf("size is not available")
			}

			var regionAvailable bool
			for _, region := range size.Regions {
				if region == c.Region {
					regionAvailable = true
					break
				}
			}

			if !regionAvailable {
				return fmt.Errorf("size %q is not available in region %q", c.Size, c.Region)
			}

			foundSize = true
			break
		}
	}
	if !foundSize {
		return fmt.Errorf("size %q not found", c.Size)
	}

	return nil
}

func ensureSSHKeysExist(service godo.KeysService, ctx context.Context, authorizedkey []byte) (string, error) {
	key, _, _, _, err := ssh.ParseAuthorizedKey(authorizedkey)
	if err != nil {
		return "", fmt.Errorf("failed to parse authorizedkey: %v", err)
	}
	fingerprint := ssh.FingerprintLegacyMD5(key)

	dokey, res, err := service.GetByFingerprint(ctx, fingerprint)
	if err != nil {
		if res != nil && res.StatusCode == http.StatusNotFound {
			dokey, _, err = service.Create(ctx, &godo.KeyCreateRequest{
				PublicKey: string(authorizedkey),
				Name:      "machine-controller",
			})
			return dokey.Fingerprint, nil
		}
		return "", fmt.Errorf("failed to get key from digitalocean: %v", err)
	}

	return dokey.Fingerprint, nil
}

func (do *digitalocean) Create(machine *v1alpha1.Machine, userdata string, authorizedkey []byte) (instance.Instance, error) {
	c, _, err := getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	ctx := context.TODO()
	client := getClient(c.Token)

	fingerprint, err := ensureSSHKeysExist(client.Keys, ctx, authorizedkey)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure ssh keys exist: %v", err)
	}

	createRequest := &godo.DropletCreateRequest{
		Image:             godo.DropletCreateImage{Slug: image},
		Name:              machine.Spec.Name,
		Region:            c.Region,
		Size:              c.Size,
		IPv6:              c.IPv6,
		PrivateNetworking: c.PrivateNetworking,
		Backups:           c.Backups,
		UserData:          userdata,
		SSHKeys:           []godo.DropletCreateSSHKey{{Fingerprint: fingerprint}},
		Tags:              c.Tags,
	}

	droplet, _, err := client.Droplets.Create(ctx, createRequest)
	if err != nil {
		return nil, err
	}
	return &doInstance{droplet: droplet}, nil
}

func (do *digitalocean) Delete(machine *v1alpha1.Machine) error {
	c, _, err := getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	ctx := context.TODO()
	client := getClient(c.Token)
	i, err := do.Get(machine)
	if err != nil {
		if err == cloudprovidererrors.InstanceNotFoundErr {
			glog.V(4).Info("instance already deleted")
			return nil
		}
		return err
	}
	doID, _ := strconv.Atoi(i.ID())
	_, err = client.Droplets.Delete(ctx, doID)
	return err
}

func (do *digitalocean) Get(machine *v1alpha1.Machine) (instance.Instance, error) {
	c, _, err := getConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	ctx := context.TODO()
	client := getClient(c.Token)
	droplets, _, err := client.Droplets.List(ctx, &godo.ListOptions{PerPage: 1000})
	if err != nil {
		return nil, fmt.Errorf("failed to get droplets: %v", err)
	}

	var d *godo.Droplet
	for _, droplet := range droplets {
		if droplet.Name == machine.Spec.Name {
			d = &droplet
		}
	}
	if d == nil {
		return nil, cloudprovidererrors.InstanceNotFoundErr
	}

	return &doInstance{droplet: d}, nil
}

type doInstance struct {
	droplet *godo.Droplet
}

func (d *doInstance) Name() string {
	return d.droplet.Name
}

func (d *doInstance) ID() string {
	return strconv.Itoa(d.droplet.ID)
}

func (d *doInstance) Status() instance.State {
	switch d.droplet.Status {
	case "new":
		return instance.InstanceStarting
	case "active":
		return instance.InstanceRunning
	default:
		return instance.InstanceStopped
	}
}

func (d *doInstance) Addresses() []string {
	var addresses []string
	for _, n := range d.droplet.Networks.V4 {
		addresses = append(addresses, n.IPAddress)
	}
	for _, n := range d.droplet.Networks.V6 {
		addresses = append(addresses, n.IPAddress)
	}
	return addresses
}
