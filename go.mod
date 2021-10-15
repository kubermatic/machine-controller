module github.com/kubermatic/machine-controller

go 1.13

require (
	cloud.google.com/go v0.73.0
	cloud.google.com/go/logging v1.1.2
	github.com/Azure/azure-sdk-for-go v49.0.0+incompatible
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.5
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/BurntSushi/toml v0.3.1
	github.com/Masterminds/semver v1.5.0
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/aliyun/alibaba-cloud-sdk-go v1.61.751
	github.com/anexia-it/go-anxcloud v0.3.8
	github.com/aws/aws-sdk-go v1.36.2
	github.com/coreos/container-linux-config-transpiler v0.9.0
	github.com/davecgh/go-spew v1.1.1
	github.com/digitalocean/godo v1.54.0
	github.com/docker/distribution v2.7.1+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/go-test/deep v1.0.7
	github.com/google/uuid v1.1.2
	github.com/gophercloud/gophercloud v0.14.0
	github.com/heptiolabs/healthcheck v0.0.0-20180807145615-6ff867650f40
	github.com/hetznercloud/hcloud-go v1.25.0
	github.com/linode/linodego v0.24.0
	github.com/packethost/packngo v0.5.1
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.1
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v1.8.0
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.7
	github.com/sethvargo/go-password v0.2.0
	github.com/tinkerbell/tink v0.0.0-20210315140655-1b178daeaeda
	github.com/vmware/govmomi v0.23.1
	golang.org/x/crypto v0.0.0-20201124201722-c8d3bf9c5392
	golang.org/x/oauth2 v0.0.0-20201109201403-9fd604954f58
	gomodules.xyz/jsonpatch/v2 v2.1.0
	google.golang.org/api v0.36.0
	google.golang.org/grpc v1.33.2
	gopkg.in/gcfg.v1 v1.2.3
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	k8c.io/operating-system-manager v0.0.0-00010101000000-000000000000
	k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver v0.19.4
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kubelet v0.19.4
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	kubevirt.io/client-go v0.30.0
	kubevirt.io/containerized-data-importer v1.10.6
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/packethost/packngo => github.com/packethost/packngo v0.1.1-0.20190410075950-a02c426e4888
	k8c.io/operating-system-manager => ../operating-system-manager
	k8s.io/client-go => k8s.io/client-go v0.20.2
)
