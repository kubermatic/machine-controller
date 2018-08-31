package vsphere

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestGetCloudConfig(t *testing.T) {
	tests := []struct {
		providerConfig []byte
		expected       string
	}{
		{providerConfig: []byte(`
{
  "cloudProviderSpec": {
    "allowInsecure": true,
    "datacenter": "Datacenter",
    "datastore": "datastore1",
    "password": "password",
    "username": "user",
    "vsphereURL": "https://your-vcenter:8443"
  }
}`),
			expected: `
[Global]
server = "your-vcenter"
port = "8443"
user = "user"
password = "password"
insecure-flag = "1" #set to 1 if the vCenter uses a self-signed cert
datastore = "datastore1"
working-dir = "/Datacenter/vm"
datacenter = "Datacenter"
`,
		},
		{providerConfig: []byte(`
{
  "cloudProviderSpec": {
    "allowInsecure": true,
    "datacenter": "Datacenter",
    "datastore": "datastore1",
    "password": "password",
    "username": "user",
    "vsphereURL": "https://your-vcenter"
  }
}`),
			expected: `
[Global]
server = "your-vcenter"
port = "443"
user = "user"
password = "password"
insecure-flag = "1" #set to 1 if the vCenter uses a self-signed cert
datastore = "datastore1"
working-dir = "/Datacenter/vm"
datacenter = "Datacenter"
`,
		},
		{providerConfig: []byte(`
{
  "cloudProviderSpec": {
    "allowInsecure": false,
    "datacenter": "Datacenter",
    "datastore": "datastore1",
    "password": "password",
    "username": "user",
    "vsphereURL": "https://your-vcenter"
  }
}`),
			expected: `
[Global]
server = "your-vcenter"
port = "443"
user = "user"
password = "password"
insecure-flag = "0" #set to 1 if the vCenter uses a self-signed cert
datastore = "datastore1"
working-dir = "/Datacenter/vm"
datacenter = "Datacenter"
`,
		},
		{providerConfig: []byte(`
{
  "cloudProviderSpec": {
    "allowInsecure": false,
    "datacenter": "Datacenter",
    "datastore": "datastore1",
    "password": "password",
    "username": "user",
    "vsphereURL": "your-vcenter"
  }
}`),
			expected: `
[Global]
server = "your-vcenter"
port = "443"
user = "user"
password = "password"
insecure-flag = "0" #set to 1 if the vCenter uses a self-signed cert
datastore = "datastore1"
working-dir = "/Datacenter/vm"
datacenter = "Datacenter"
`,
		},
		{providerConfig: []byte(`
{
  "cloudProviderSpec": {
    "allowInsecure": false,
    "datacenter": "Datacenter",
    "folder": "/Datacenter/vm/custom-folder",
    "datastore": "datastore1",
    "password": "password",
    "username": "user",
    "vsphereURL": "your-vcenter"
  }
}`),
			expected: `
[Global]
server = "your-vcenter"
port = "443"
user = "user"
password = "password"
insecure-flag = "0" #set to 1 if the vCenter uses a self-signed cert
datastore = "datastore1"
working-dir = "/Datacenter/vm/custom-folder"
datacenter = "Datacenter"
`,
		},
	}

	p := provider{}
	for _, test := range tests {
		providerconfig := v1alpha1.ProviderConfig{Value: &runtime.RawExtension{Raw: test.providerConfig}}
		machineSpec := v1alpha1.MachineSpec{ProviderConfig: providerconfig}
		cloudConfig, _, err := p.GetCloudConfig(machineSpec)
		if err != nil {
			t.Fatalf("Error rendering cloud-config: %v", err)
		}
		if cloudConfig != test.expected {
			t.Errorf("Cloud config was not as expected! Result: `%s`, Expected: `%s`",
				cloudConfig, test.expected)
		}
	}
}
