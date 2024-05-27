# Cloud providers

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
vpcId: "vpc-079f7648481a11e77"
# subnet id for the instance
subnetId: "subnet-2bff4f43"
# enable public IP assignment, default is true
assignPublicIP: true
# instance type
instanceType: "t2.micro"
# enable provisioning as spot instance machine, default false
isSpotInstance: false
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
# When not set a 'kubernetes-v1' security group will get created
securityGroupIDs:
- ""
# name of the instance profile to use, required.
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
# application Credential ID and Secret can be used in place of username, password, tenantName/tenantID, and domainName.
# application credentials ID
applicationCredentialID: ""
# application credentials secret
applicationCredentialSecret: ""
# your openstack username
username: ""
# your openstack password
password: ""
# the openstack domain
domainName: "default"
# project name
projectName: ""
# project id
projectID: ""
# tenant name (deprecated, should use projectName)
tenantName: ""
# tenant Id (deprecated, should use projectID)
tenantID: ""
# image to use (currently only ubuntu is supported)
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
# compute microversion
computeAPIVersion: ""
# set trust-device-path flag for kubelet
trustDevicePath: false
# set root disk size
rootDiskSizeGB: 50
# set root disk volume type
rootDiskVolumeType: ""
# set node-volume-attach-limit flag for cloud-config
nodeVolumeAttachLimit: 20
# the list of tags you would like to attach to the instance
tags:
  tagKey: tagValue
```
