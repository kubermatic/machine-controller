# Kubermatic machine-controller

**Important Note: User data plugins for machine-controller are deprecated and will soon be removed. [Operating System Manager](https://github.com/kubermatic/operating-system-manager) is the successor of user data plugins. It's responsible for creating and managing the required configurations for worker nodes in a Kubernetes cluster with better modularity and extensibility. Please refer to [Operating System Manager][8] for more details.**

## Table of Contents

- [Kubermatic machine-controller](#kubermatic-machine-controller)
  - [Table of Contents](#table-of-contents)
  - [Features](#features)
    - [What Works](#what-works)
    - [Supported Kubernetes Versions](#supported-kubernetes-versions)
    - [Community Providers](#community-providers)
  - [What doesn't Work](#what-doesnt-work)
  - [Quickstart](#quickstart)
    - [Deploy machine-controller](#deploy-the-machine-controller)
    - [Creating a MachineDeployment](#creating-a-machinedeployment)
  - [Advanced Usage](#advanced-usage)
    - [Specifying the Apiserver Endpoint](#specifying-the-apiserver-endpoint)
    - [CA Data](#ca-data)
    - [Apiserver Endpoint](#apiserver-endpoint)
      - [Example cluster-info ConfigMap](#example-cluster-info-configmap)
  - [Development](#development)
    - [Testing](#testing)
      - [Unit Tests](#unit-tests)
      - [End-to-End Locally](#end-to-end-locally)
  - [Troubleshooting](#troubleshooting)
  - [Contributing](#contributing)
    - [Before You Start](#before-you-start)
    - [Pull Requests](#pull-requests)
  - [Changelog](#changelog)

## Features

### What Works

- Creation of worker nodes on AWS, Digitalocean, Openstack, Azure, Google Cloud Platform, Nutanix, VMWare Cloud Director, VMWare vSphere, Hetzner Cloud and Kubevirt
- Using Ubuntu, Flatcar, CentOS 7 or Rocky Linux 8 distributions ([not all distributions work on all providers](/docs/operating-system.md))

### Supported Kubernetes Versions

machine-controller tries to follow the Kubernetes version
[support policy](https://kubernetes.io/docs/setup/release/version-skew-policy/) as close as possible.

Currently supported K8S versions are:

- 1.27
- 1.26
- 1.25
- 1.24

### Community Providers

Some cloud providers implemented in machine-controller have been graciously contributed by community members. Those cloud providers are not part of the automated end-to-end
tests run by the machine-controller developers and thus, their status cannot be guaranteed. The machine-controller developers assume that they are functional, but can only
offer limited support for new features or bugfixes in those providers.

The current list of community providers is:

- Linode
- Vultr
- OpenNebula

## What Doesn't Work

- Master creation (Not planned at the moment)

## Quickstart

### Deploy machine-controller

- Install [cert-manager](https://cert-manager.io/) for generating certificates used by webhooks since they serve using HTTPS

```terminal
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.11.2/cert-manager.yaml
```

- Run `kubectl apply -f examples/operating-system-manager.yaml` to deploy the operating-system-manager which is responsible for managing user data for worker machines.
- Run `kubectl apply -f examples/machine-controller.yaml` to deploy the machine-controller.

### Creating a `MachineDeployment`

```bash
# edit examples/$cloudprovider-machinedeployment.yaml & create the machineDeployment
kubectl create -f examples/$cloudprovider-machinedeployment.yaml
```

## Advanced Usage

### Specifying the Apiserver Endpoint

By default the controller looks for a `cluster-info` ConfigMap within the `kube-public` Namespace.
If one is found which contains a minimal kubeconfig (kubeadm cluster have them by default), this kubeconfig will be used for the node bootstrapping.
The kubeconfig only needs to contain two things:

- CA-Data
- The public endpoint for the Apiserver

If no ConfigMap can be found:

### CA Data

The Certificate Authority (CA) will be loaded from the passed kubeconfig when running outside the cluster or from `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` when running inside the cluster.

### Apiserver Endpoint

The first endpoint from the kubernetes endpoints will be taken. `kubectl get endpoints kubernetes -o yaml`

#### Example cluster-info ConfigMap

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

## Development

### Testing

#### Unit Tests

Simply run `make test-unit`

#### End-to-End Locally

**_[WIP]_**

## Troubleshooting

If you encounter issues [file an issue][1] or talk to us on the [#kubermatic channel][2] on the [Kubermatic Slack][3].

## Contributing

Thanks for taking the time to join our community and start contributing!

### Before You Start

- Please familiarize yourself with the [Code of Conduct][4] before contributing.
- See [CONTRIBUTING.md][5] for instructions on the developer certificate of origin that we require.
- Read how [we're using ZenHub][6] for project and roadmap planning

### Pull Requests

- We welcome pull requests. Feel free to dig through the [issues][1] and jump in.

## Changelog

See [the list of releases][7] to find out about feature changes.

[1]: https://github.com/kubermatic/machine-controller/issues
[2]: https://kubermatic.slack.com/messages/kubermatic
[3]: http://slack.kubermatic.io/
[4]: code-of-conduct.md
[5]: CONTRIBUTING.md
[6]: Zenhub.md
[7]: https://github.com/kubermatic/machine-controller/releases
[8]: https://docs.kubermatic.com/operatingsystemmanager