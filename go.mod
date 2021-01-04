module github.com/kubermatic/machine-controller

go 1.13

require (
	cloud.google.com/go v0.73.0
	cloud.google.com/go/logging v1.1.2
	github.com/Azure/azure-sdk-for-go v49.0.0+incompatible
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.5
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/Azure/go-autorest/autorest/validation v0.3.0 // indirect
	github.com/Masterminds/semver v1.5.0
	github.com/Masterminds/sprig/v3 v3.1.0
	github.com/ajeddeloh/go-json v0.0.0-20170920214419-6a2fe990e083 // indirect
	github.com/ajeddeloh/yaml v0.0.0-20170912190910-6b94386aeefd // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v1.61.751
	github.com/anexia-it/go-anxcloud v0.3.8
	github.com/aws/aws-sdk-go v1.36.2
	github.com/coreos/container-linux-config-transpiler v0.9.0
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/ignition v0.35.0 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/digitalocean/godo v1.54.0
	github.com/docker/distribution v2.7.1+incompatible
	github.com/emicklei/go-restful v2.11.2+incompatible // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/go-openapi/spec v0.19.6 // indirect
	github.com/go-openapi/swag v0.19.7 // indirect
	github.com/go-test/deep v1.0.7
	github.com/google/uuid v1.1.2
	github.com/gophercloud/gophercloud v0.14.0
	github.com/heptiolabs/healthcheck v0.0.0-20180807145615-6ff867650f40
	github.com/hetznercloud/hcloud-go v1.23.1
	github.com/linode/linodego v0.24.0
	github.com/mailru/easyjson v0.7.1 // indirect
	github.com/mattbaird/jsonpatch v0.0.0-20200820163806-098863c1fc24
	github.com/oklog/run v1.1.0
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/packethost/packngo v0.1.1-0.20190410075950-a02c426e4888
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.1
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v1.8.0
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.7
	github.com/sethvargo/go-password v0.2.0
	github.com/vincent-petithory/dataurl v0.0.0-20160330182126-9a301d65acbb // indirect
	github.com/vmware/govmomi v0.23.1
	go.uber.org/atomic v1.5.1 // indirect
	go.uber.org/multierr v1.4.0 // indirect
	go4.org v0.0.0-20200104003542-c7e774b10ea0 // indirect
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
	golang.org/x/oauth2 v0.0.0-20201109201403-9fd604954f58
	google.golang.org/api v0.36.0
	gopkg.in/DATA-DOG/go-sqlmock.v1 v1.3.0 // indirect
	gopkg.in/gcfg.v1 v1.2.3
	gopkg.in/ini.v1 v1.46.0 // indirect
	k8s.io/api v0.19.4
	k8s.io/apiextensions-apiserver v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kubelet v0.19.4
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	kubevirt.io/client-go v0.30.0
	kubevirt.io/containerized-data-importer v1.10.6
	sigs.k8s.io/controller-runtime v0.6.4
	sigs.k8s.io/yaml v1.2.0
)

replace k8s.io/client-go => k8s.io/client-go v0.19.4
