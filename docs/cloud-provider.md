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
# instance type
instanceType: "t2.micro"
# size of the root disk in gb
diskSize: 50
# root disk type (gp2, io1, st1, sc1, or standard)
diskType: "gp2"
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
```

## Google Cloud Platform

### machine.spec.providerConfig.cloudProviderSpec

```yaml
clientID: "<< GOOGLE_CLIENT_ID >>"
projectID: "<< GOOGLE_PROJECT_ID >>"
email: "<< GOOGLE_EMAIL >>"
privateKey: "<< GOOGLE_PRIVATE_KEY >>"
# See https://cloud.google.com/compute/docs/regions-zones/
zone: "europe-west3-a"
# See https://cloud.google.com/compute/docs/machine-types
machineType: "n1-standard-2"
# In GB
diskSize: 50
# Can be 'pd-standard' or 'pd-ssd'
diskType: "pd-standard"
labels:
    "KubernetesCluster": "my-cluster"            
```

## Hetzner cloud

### machine.spec.providerConfig.cloudProviderSpec
```yaml
  token: "<< HETZNER_API_TOKEN >>"
  serverType: "cx11"
  datacenter: ""
  location: "fsn1"
```

## Linode

### machine.spec.providerConfig.cloudProviderSpec
```yaml
# your linode token
token: "<< YOUR_LINODE_TOKEN >>"
# droplet region
region: "eu-west"
# droplet size
type: "g6-standard-2"
# enable backups for the linode
backups: false
# enable private networking for the linode
private_networking: true
# add the following tags to the linode
tags:
- "machine-controller"
```

