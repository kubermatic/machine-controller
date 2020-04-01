# Cloud providers

## Digitalocean

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# your digitalocean token
token: "<< YOUR_DO_TOKEN >>"
# droplet region
region: "fra1"
# droplet size
size: "2gb"
# enable backups for the droplet
backups: false
# enable ipv6 for the droplet
ipv6: false- Add operating system config
# enable private networking for the droplet
private_networking: true
# enable monitoring for the droplet
monitoring: true
# add the following tags to the droplet
tags:
- "machine-controller"
```

## AWS

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# your aws access key id
accessKeyId: "<< YOUR_ACCESS_KEY_ID >>"
# your aws secret access key id
secretAccessKey: "<< YOUR_SECRET_ACCESS_KEY_ID >>"
# region for the instance
region: "eu-central-1"
# avaiability zone for the instance
availabilityZone: "eu-central-1a"
# vpc id for the instance
vpcId: "vpc-819f62e9"
# subnet id for the instance
subnetId: "subnet-2bff4f43"
# enable public IP assignment, default is true
assignPublicIP: true
# instance type
instanceType: "t2.micro"
# size of the root disk in gb
diskSize: 50
# root disk type (gp2, io1, st1, sc1, or standard)
diskType: "gp2"
# IOPS for EBS volumes, required with diskType: io1
diskIops: 500
# enable EBS volume encryption
ebsVolumeEncrypted: false
# optional! the ami id to use. Needs to fit to the specified operating system
ami: ""
# optional! The security group ids for the instance.
# When not set a 'kubernetes-v1' security gruop will get created
securityGroupIDs:
- ""
# name of the instance profile to use.
# When not set a 'kubernetes-v1' instance profile will get created
instanceProfile : ""

# instance tags ("KubernetesCluster": "my-cluster" is a required tag.
# If not set, the kubernetes controller-manager will delete the nodes)
tags:
  "KubernetesCluster": "my-cluster"
```
## Openstack

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# identity endpoint of your openstack installation
identityEndpoint: ""
# your openstack username
username: ""
# your openstack password
password: ""
# the openstack domain
domainName: "default"
# tenant name
tenantName: ""
# image to use (currently only ubuntu & coreos are supported)
image: "Ubuntu 18.04 amd64"
# instance flavor
flavor: ""
# additional security groups.
# a default security group will be created which node-to-node communication
securityGroups:
- "external-ssh"
# the name of the subnet to use
subnet: ""
# [not implemented] the floating ip pool to use. When set a floating ip will be assigned o the instance
floatingIpPool: ""
# the availability zone to create the instance in
availabilityZone: ""
# the region to operate in
region: ""
# the name of the network to use
network: ""
# set trust-device-path flag for kubelet
trustDevicePath: false
# set root disk size
rootDiskSizeGB: 50
# set node-volume-attach-limit flag for cloud-config
nodeVolumeAttachLimit: 20
# the list of tags you would like to attach to the instance
tags:
  tagKey: tagValue
```

## Google Cloud Platform

### machine.spec.providerConfig.cloudProviderSpec

```yaml
serviceAccount: "<< GOOGLE_SERVICE_ACCOUNT >>"
# See https://cloud.google.com/compute/docs/regions-zones/
zone: "europe-west3-a"
# See https://cloud.google.com/compute/docs/machine-types
machineType: "n1-standard-2"
# See https://cloud.google.com/compute/docs/instances/preemptible
preemptible: false
# In GB
diskSize: 25
# Can be 'pd-standard' or 'pd-ssd'
diskType: "pd-standard"
# The name or self_link of the network and subnetwork to attach this interface to;
# either of both can be provided, otherwise default network will taken
# in case if both empty — default network will be used
network: "my-cool-network"
subnetwork: "my-cool-subnetwork"
# assign a public IP Address. Required for Internet access
assignPublicIPAddress: true
# set node labels
labels:
    "kubernetesCluster": "my-cluster"
```

## Hetzner cloud

### machine.spec.providerConfig.cloudProviderSpec
```yaml
  token: "<< HETZNER_API_TOKEN >>"
  serverType: "cx11"
  datacenter: ""
  location: "fsn1"
  # Optional: network IDs or names
  networks:
    - "<< YOUR_NETWORK >>"
  # set node labels
  labels:
    "kubernetesCluster": "my-cluster"
```

## Linode

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# your linode token
token: "<< YOUR_LINODE_TOKEN >>"
# linode region
region: "eu-west"
# linode size
type: "g6-standard-2"
# enable backups for the linode
backups: false
# enable private networking for the linode
private_networking: true
# add the following tags to the linode
tags:
- "machine-controller"
```

## Alibaba

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# If empty, can be set via ALIBABA_ACCESS_KEY_ID env var
  accessKeyID: "<< YOUR ACCESS ID >>"
  accessKeySecret: "<< YOUR ACCESS SECRET >>"
  # instance type
  instanceType: "ecs.t1.xsmall"
  # instance name
  instanceName: "alibaba-instance"
  # region
  regionID: eu-central-1
  # image id
  imageID: "aliyun_2_1903_64_20G_alibase_20190829.vhd"
  # disk type
  diskType: "cloud_efficiency"
  # disk size in GB
  diskSize: "40"
  # set an existing vSwitch ID to use, VPC default is used if not set.
  vSwitchID:
  labels:
    "kubernetesCluster": "my-cluster"
```


## Azure

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# Can also be set via the env var 'AZURE_TENANT_ID' on the machine-controller
tenantID: "<< AZURE_TENANT_ID >>"
# Can also be set via the env var 'AZURE_CLIENT_ID' on the machine-controller
clientID: "<< AZURE_CLIENT_ID >>"
# Can also be set via the env var 'AZURE_CLIENT_SECRET' on the machine-controller
clientSecret: "<< AZURE_CLIENT_SECRET >>"
# Can also be set via the env var 'AZURE_SUBSCRIPTION_ID' on the machine-controller
subscriptionID: "<< AZURE_SUBSCRIPTION_ID >>"
# Azure location
location: "westeurope"
# Azure resource group
resourceGroup: "<< YOUR_RESOURCE_GROUP >>"
# Azure availability set
availabilitySet: "<< YOUR AVAILABILITY SET >>"
# VM size
vmSize: "Standard_B1ms"
# optional OS and Data disk size values in GB. If not set, the defaults for the vmSize will be used.
osDiskSize: 30
dataDiskSize: 30
# network name
vnetName: "<< VNET_NAME >>"
# subnet name
subnetName: "<< SUBNET_NAME >>"
# route able name
routeTableName: "<< ROUTE_TABLE_NAME >>"
# assign public IP addresses for nodes, required for Internet access
assignPublicIP: true
# security group
securityGroupName: my-security-group
# node tags
tags:
  "kubernetesCluster": "my-cluster"
```

## KubeVirt

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# kubeconfig to access KubeVirt cluster
kubeconfig: '<< KUBECONFIG >>'
# KubeVirt namespace
namespace: kube-system
# kubernetes storage class
storageClassName: kubermatic-fast
# storage PVC size
pvcSize: "10Gi"
# OS Image URL
sourceURL: http://10.109.79.210/<< OS_NAME >>.img
# instance resources
cpus: "1"
memory: "2048M"
```

## Packet

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# If empty, can be set via PACKET_API_KEY env var
apiKey: "<< PACKET_API_KEY >>"
# instance type
instanceType: "t1.small.x86"
# packet project ID
projectID: "<< PROJECT_ID >>"
# packet facilities
facilities:
  - "ewr1"
# packet billingCycle
billingCycle: ""
# node tags
tags:
  "kubernetesCluster": "my-cluster"
```

## vSphere

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# Can also be set via the env var 'VSPHERE_USERNAME' on the machine-controller
username: '<< VSPHERE_USERNAME >>'
# Can also be set via the env var 'VSPHERE_ADDRESS' on the machine-controller
# example: 'https://your-vcenter:8443'. '/sdk' gets appended automatically
vsphereURL: '<< VSPHERE_ADDRESS >>'
# Can also be set via the env var 'VSPHERE_PASSWORD' on the machine-controller
password: "<< VSPHERE_PASSWORD >>"
# datacenter name
datacenter: datacenter1
# VM template name
templateVMName: ubuntu-template
# Optional. Sets the networks on the VM. If no network is specified, the template default will be used.
vmNetName: network1
# Optional
folder: folder1
cluster: cluster1
# either datastore or datastoreCluster have to be provided.
datastore: datastore1
datastoreCluster: datastore-cluster1
# Can also be set via the env var 'VSPHERE_ALLOW_INSECURE' on the machine-controller
allowInsecure: true
# instance resources
cpus: 2
memoryMB: 2048
# Optional: Resize the root disk to this size. Must be bigger than the existing size
# Default is to leave the disk at the same size as the template
diskSizeGB: 10
```
