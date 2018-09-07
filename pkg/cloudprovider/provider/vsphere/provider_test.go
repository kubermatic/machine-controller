package vsphere

import (
	"testing"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
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
			expected: `[Global]
user          = user
password      = password
port          = 8443
insecure-flag = true

[Disk]
scsicontrollertype = pvscsi

[Workspace]
server            = your-vcenter
datacenter        = Datacenter
folder            = /Datacenter/vm
default-datastore = datastore1
resourcepool-path = 

[VirtualCenter "your-vcenter"]
user        = user
password    = password
port        = 8443
datacenters = Datacenter

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
			expected: `[Global]
user          = user
password      = password
port          = 
insecure-flag = true

[Disk]
scsicontrollertype = pvscsi

[Workspace]
server            = your-vcenter
datacenter        = Datacenter
folder            = /Datacenter/vm
default-datastore = datastore1
resourcepool-path = 

[VirtualCenter "your-vcenter"]
user        = user
password    = password
port        = 
datacenters = Datacenter

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
			expected: `[Global]
user          = user
password      = password
port          = 
insecure-flag = false

[Disk]
scsicontrollertype = pvscsi

[Workspace]
server            = your-vcenter
datacenter        = Datacenter
folder            = /Datacenter/vm
default-datastore = datastore1
resourcepool-path = 

[VirtualCenter "your-vcenter"]
user        = user
password    = password
port        = 
datacenters = Datacenter

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
			expected: `[Global]
user          = user
password      = password
port          = 
insecure-flag = false

[Disk]
scsicontrollertype = pvscsi

[Workspace]
server            = your-vcenter
datacenter        = Datacenter
folder            = /Datacenter/vm/custom-folder
default-datastore = datastore1
resourcepool-path = 

[VirtualCenter "your-vcenter"]
user        = user
password    = password
port        = 
datacenters = Datacenter

`,
		},
	}

	p := provider{}
	for _, test := range tests {
		providerconfigRuntimeRawExtension := runtime.RawExtension{Raw: test.providerConfig}
		machineSpec := v1alpha1.MachineSpec{ProviderConfig: providerconfigRuntimeRawExtension}
		cloudConfig, _, err := p.GetCloudConfig(machineSpec)
		if err != nil {
			t.Fatalf("Error rendering cloud-config: %v", err)
		}

		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(test.expected)),
			B:        difflib.SplitLines(cloudConfig),
			FromFile: "Expected",
			ToFile:   "Current",
			Context:  3,
		}
		diffStr, err := difflib.GetUnifiedDiffString(diff)
		if err != nil {
			t.Error(err)
		}

		if diffStr != "" {
			t.Errorf("got diff between expected and actual result: \n%s\n", diffStr)
		}
	}
}
