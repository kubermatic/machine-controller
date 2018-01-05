# machine-controller

# Building
```bash
go build -i github.com/kubermatic/machine-controller/cmd/controller
```

# Running
```bash
./controller -logtostderr -v=8 -kubeconfig=/path/to/kubeconfig
```

# Features
## What works (tested under kubernetes v1.8.5)
- Creation of worker nodes on AWS & Digitalocean & Openstack
  - AWS
    - Creation of a security group
    - Creation of a instance profile
- Using Ubuntu & Coreos as operating system

## What does not work
- Master creation
- Installation of a container runtime based on `machine.spec.versions.containerRuntime`. Currently only latest docker will be used.
- Probably a lot, this is a very simple implementation.

# Requirements
The controller expects a cluster-info configmap to exist within the kube-public namespace.
This configmap needs to contain a kubeconfig without credentials.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-info
  namespace: kube-public
data:
  kubeconfig: |
    apiVersion: v1
    clusters:
    - cluster:
        certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURHRENDQWdDZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREE5TVRzd09RWURWUVFERXpKeWIyOTAKTFdOaExtaG1kblEwWkd0bllpNWxkWEp2Y0dVdGQyVnpkRE10WXk1a1pYWXVhM1ZpWlhKdFlYUnBZeTVwYnpBZQpGdzB4TnpFeU1qSXdPVFUyTkROYUZ3MHlOekV5TWpBd09UVTJORE5hTUQweE96QTVCZ05WQkFNVE1uSnZiM1F0ClkyRXVhR1oyZERSa2EyZGlMbVYxY205d1pTMTNaWE4wTXkxakxtUmxkaTVyZFdKbGNtMWhkR2xqTG1sdk1JSUIKSWpBTkJna3Foa2lHOXcwQkFRRUZBQU9DQVE4QU1JSUJDZ0tDQVFFQTNPMFZBZm1wcHM4NU5KMFJ6ckhFODBQTQo0cldvRk9iRXpFWVQ1Unc2TjJ0V3lqazRvMk5KY1R1YmQ4bUlONjRqUjFTQmNQWTB0ZVRlM2tUbEx0OWMrbTVZCmRVZVpXRXZMcHJoMFF5YjVMK0RjWDdFZG94aysvbzVIL0txQW1VT0I5TnR1L2VSM0EzZ0xxNHIvdnFpRm1yTUgKUUxHbllHNVVPN25WSmc2RmJYbGxtcmhPWlUvNXA3c0xwQUpFbCtta3RJbzkybVA5VGFySXFZWTZTblZTSmpDVgpPYk4zTEtxU0gxNnFzR2ZhclluZUl6OWJGKzVjQTlFMzQ1cFdQVVhveXFRUURSNU1MRW9NY0tzYVF1V2g3Z2xBClY3SUdYUzRvaU5HNjhDOXd5REtDd3B2NENkbGJxdVRPMVhDb2puS1o0OEpMaGhFVHRxR2hIa2xMSkEwVXpRSUQKQVFBQm95TXdJVEFPQmdOVkhROEJBZjhFQkFNQ0FxUXdEd1lEVlIwVEFRSC9CQVV3QXdFQi96QU5CZ2txaGtpRwo5dzBCQVFzRkFBT0NBUUVBamlNU0kxTS9VcUR5ZkcyTDF5dGltVlpuclBrbFVIOVQySVZDZXp2OUhCUG9NRnFDCmpENk5JWVdUQWxVZXgwUXFQSjc1bnNWcXB0S0loaTRhYkgyRnlSRWhxTG9DOWcrMU1PZy95L1FsM3pReUlaWjIKTysyZGduSDNveXU0RjRldFBXamE3ZlNCNjF4dS95blhyZG5JNmlSUjFaL2FzcmJxUXd5ZUgwRjY4TXd1WUVBeQphMUNJNXk5Q1RmdHhxY2ZpNldOTERGWURLRXZwREt6aXJ1K2xDeFJWNzNJOGljWi9Tbk83c3VWa0xUNnoxcFBRCnlOby9zNXc3Ynp4ekFPdmFiWTVsa2VkVFNLKzAxSnZHby9LY3hsaTVoZ1NiMWVyOUR0VERXRjdHZjA5ZmdpWlcKcUd1NUZOOUFoamZodTZFcFVkMTRmdXVtQ2ttRHZIaDJ2dzhvL1E9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==
        server: https://hfvt4dkgb.europe-west3-c.dev.kubermatic.io:30002
      name: ""
    contexts: []
    current-context: ""
    kind: Config
    preferences: {}
    users: []
```

# Creating a machine
```bash
# edit examples/machine.yaml & create the machine 
kubectl create -f examples/machine.yaml
```

# Providers

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
image: "Ubuntu 16.04 amd64"
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
