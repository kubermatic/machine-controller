module github.com/kubermatic/machine-controller

go 1.13

require (
	cloud.google.com/go v0.38.0
	github.com/Azure/azure-sdk-for-go v31.1.0+incompatible
	github.com/Azure/go-autorest/autorest v0.9.4 // indirect
	github.com/Azure/go-autorest/autorest/azure/auth v0.4.2
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/Masterminds/semver v1.4.2
	github.com/Masterminds/sprig v2.15.0+incompatible
	github.com/ajeddeloh/go-json v0.0.0-20170920214419-6a2fe990e083 // indirect
	github.com/ajeddeloh/yaml v0.0.0-20170912190910-6b94386aeefd // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v0.0.0-20190828035149-111b102694f9
	github.com/anexia-it/go-anxcloud v0.1.0
	github.com/aokoli/goutils v1.0.1 // indirect
	github.com/aws/aws-sdk-go v1.19.25
	github.com/coreos/container-linux-config-transpiler v0.9.0
	github.com/coreos/etcd v3.3.17+incompatible
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/ignition v0.24.0 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/digitalocean/godo v1.1.3
	github.com/docker/distribution v2.7.1+incompatible
	github.com/emicklei/go-restful v2.11.2+incompatible // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-openapi/jsonreference v0.19.3 // indirect
	github.com/go-openapi/spec v0.19.6 // indirect
	github.com/go-openapi/swag v0.19.7 // indirect
	github.com/go-test/deep v1.0.1
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/uuid v1.1.1
	github.com/googleapis/gnostic v0.4.0 // indirect
	github.com/gophercloud/gophercloud v0.2.1-0.20190626201551-2949719e8258
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/heptiolabs/healthcheck v0.0.0-20180807145615-6ff867650f40
	github.com/hetznercloud/hcloud-go v1.15.1
	github.com/huandu/xstrings v1.0.0 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/linode/linodego v0.7.1
	github.com/mailru/easyjson v0.7.1 // indirect
	github.com/mattbaird/jsonpatch v0.0.0-20171005235357-81af80346b1a
	github.com/oklog/run v1.0.0
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/packethost/packngo v0.1.1-0.20190410075950-a02c426e4888
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pborman/uuid v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v1.4.1
	github.com/prometheus/procfs v0.0.10 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.7
	github.com/sethvargo/go-password v0.1.2
	github.com/tent/http-link-go v0.0.0-20130702225549-ac974c61c2f9 // indirect
	github.com/vincent-petithory/dataurl v0.0.0-20160330182126-9a301d65acbb // indirect
	github.com/vmware/govmomi v0.22.2
	go.uber.org/atomic v1.5.1 // indirect
	go.uber.org/multierr v1.4.0 // indirect
	go.uber.org/zap v1.13.0 // indirect
	go4.org v0.0.0-20200104003542-c7e774b10ea0 // indirect
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
	golang.org/x/lint v0.0.0-20200130185559-910be7a94367 // indirect
	golang.org/x/mod v0.2.0 // indirect
	golang.org/x/net v0.0.0-20200202094626-16171245cfb2 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sys v0.0.0-20200217220822-9197077df867 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	golang.org/x/tools v0.0.0-20200219054238-753a1d49df85 // indirect
	google.golang.org/api v0.6.0
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20200212174721-66ed5ce911ce // indirect
	google.golang.org/grpc v1.27.1 // indirect
	gopkg.in/DATA-DOG/go-sqlmock.v1 v1.3.0 // indirect
	gopkg.in/gcfg.v1 v1.2.3
	gopkg.in/ini.v1 v1.46.0 // indirect
	gopkg.in/resty.v1 v1.12.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.16.4
	k8s.io/apiextensions-apiserver v0.16.4
	k8s.io/apimachinery v0.16.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200204173128-addea2498afe // indirect
	k8s.io/kubelet v0.16.4
	k8s.io/utils v0.0.0-20200124190032-861946025e34
	kubevirt.io/client-go v0.30.0
	kubevirt.io/containerized-data-importer v1.10.6
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 // indirect
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.3.0+incompatible
	github.com/coreos/prometheus-operator => github.com/coreos/prometheus-operator v0.28.0
	k8s.io/client-go => k8s.io/client-go v0.16.4
)
