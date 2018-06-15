package vsphere

import (
	"testing"

	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestGetCloudConfig(t *testing.T) {
	p := provider{}
	providerConfigRaw := []byte(`
{
  "cloudProvider": "vsphere",
  "cloudProviderSpec": {
    "MemoryMB": 2048,
    "allowInsecure": true,
    "cluster": "test-cluster",
    "cpus": 2,
    "datacenter": "Datacenter",
    "datastore": "datastore1",
    "password": "password",
    "templateVMName": "ubuntu-template",
    "username": "user",
    "vsphereURL": "https://your-vcenter:8443"
  }
}`)
	providerconfigRuntimeRawExtension := runtime.RawExtension{Raw: providerConfigRaw}
	machineSpec := v1alpha1.MachineSpec{ProviderConfig: providerconfigRuntimeRawExtension}
	cloudConfig, _, err := p.GetCloudConfig(machineSpec)
	if err != nil {
		t.Fatalf("Error rendering cloud-config: %v", err)
	}
	expected := `
[Global]
server = "your-vcenter"
port = "8443"
user = "user"
password = "password"
insecure-flag = "1" #set to 1 if the vCenter uses a self-signed cert
datastore = "datastore1"
working-dir = "/Datacenter/vm"
datacenter = "Datacenter"`
	if cloudConfig != expected {
		t.Errorf("Cloud config was not as expected! Result: `%s`, Expected: `%s`", cloudConfig, expected)
	}
}
