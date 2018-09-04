# Container Runtimes

## Defaulting
The machine-controller is able to default to a supported container runtime in case no runtime was specified in the machine-spec.
Also when no specific container runtime version is defined, the controller will try to default to a version.

Having a machine like the following:
```yaml
apiVersion: "machine.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine-docker
spec:
  metadata:
    name: node-docker
  providerConfig:
    sshPublicKeys:
     - "some-ssh-pub-key"
    cloudProvider: "digitalocean"
    cloudProviderSpec:
      token: "some-do-token"
      region: "fra1"
      size: "2gb"
      backups: false
      ipv6: false
      private_networking: true
      monitoring: false
      tags:
       - "machine-controller"
    operatingSystem: "ubuntu"
    operatingSystemSpec:
      distUpgradeOnBoot: false
  roles:
 - "Node"
  versions:
    kubelet: "1.9.2"
    containerRuntime:
      name: "docker"
      version: ""
```

The machine-controller would default to Docker in version 1.13.1 as it is the supported version for kubernetes 1.9:

```yaml
apiVersion: "machine.k8s.io/v1alpha1"
kind: Machine
metadata:
  name: machine-docker
spec:
  metadata:
    name: node-docker
  providerConfig:
    sshPublicKeys:
     - "some-ssh-pub-key"
    cloudProvider: "digitalocean"
    cloudProviderSpec:
      token: "some-do-token"
      region: "fra1"
      size: "2gb"
      backups: false
      ipv6: false
      private_networking: true
      monitoring: false
      tags:
       - "machine-controller"
    operatingSystem: "ubuntu"
    operatingSystemSpec:
      distUpgradeOnBoot: false
  roles:
 - "Node"
  versions:
    kubelet: "1.9.2"
    containerRuntime:
      name: "docker"
      version: "1.13.1"
```

## Available runtimes

### Ubuntu

#### Docker
- 17.12 / 17.12.1
- 18.03 / 18.03.1
- 18.06.0
- 18.06 / 18.06.1

### Container Linux

#### Docker
The different docker version are supported via the torcx flag describe in https://coreos.com/blog/toward-docker-17-in-container-linux

- 17.09 / 17.09.1
- 1.12 / 1.12.6

### CentOS

#### Docker

- 1.13 / 1.13.1
