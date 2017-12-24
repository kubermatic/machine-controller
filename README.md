# machine-controller

# Building
```bash
go build -i github.com/kubermatic/machine-controller/cmd/controller
```

# Running
```bash
./controller -logtostderr -v=8 -kubeconfig=/path/to/kubeconfig
```

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
# Your Digitalocean token
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

# TODO
- Make operating system configurable. `machine.spec.providerConfig.operatingSystem`
Currently we only support coreos but we need to support ubuntu as well. 
Based on this field we know how to generate the userdata / cloud-init.
- Support Ubuntu as OS. (Should support `machine.spec.versions.containerRuntime`) - i want CRI-O!
- Add AWS provider
- Add Openstack provider
