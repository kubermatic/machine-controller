# Kubermatic machine-controller

# Table of Contents

- [Features](#features)
- [Quickstart](#Quickstart)
  - [Deployment](#Deploy-the-machine-controller)
  - [Creating a machineDeployment](#Creating-a-machineDeployment)
  - [Special network restrictions](/docs/network-restrictions.md)
- [Cloud provider](/docs/cloud-provider.md)
- [Operating system](/docs/operating-system.md)
  - [OpenStack images](/docs/openstack-images.md)
- [Development](#development)
- [How to add a new provider](docs/howto-provider.md)
- [E2E Infra](/docs/e2e-infra.md)
- [TroubleShooting](#troubleshooting)
- [Contributing](#contributing)
- [Changelog](#changelog)

# Features
## What works
- Creation of worker nodes on AWS, Digitalocean, Openstack, Azure, Google Cloud Platform, VMWare Vsphere, Linode, Hetzner cloud and Kubevirt (experimental)
- Using Ubuntu, Flatcar or CentOS 7 distributions ([not all distributions work on all providers](/docs/operating-system.md))

## Supported Kubernetes versions
machine-controller tries to follow the Kubernetes version
[support policy](https://kubernetes.io/docs/setup/release/version-skew-policy/) as close as possible.

Currently supported K8S versions are:

- 1.23
- 1.22
- 1.21
- 1.20

## What does not work
- Master creation (Not planned at the moment)

# Quickstart

## Deploy the machine-controller

`make deploy`

## Creating a machineDeployment
```bash
# edit examples/$cloudprovider-machinedeployment.yaml & create the machineDeployment
kubectl create -f examples/$cloudprovider-machinedeployment.yaml
```

## Advanced usage

### Specifying the apiserver endpoint
By default the controller looks for a `cluster-info` ConfigMap within the `kube-public` Namespace.
If one is found which contains a minimal kubeconfig (kubeadm cluster have them by default), this kubeconfig will be used for the node bootstrapping.
The kubeconfig only needs to contain two things:
- CA-Data
- The public endpoint for the Apiserver

If no ConfigMap can be found:

**CA-data**

The CA will be loaded from the passed kubeconfig when running outside the cluster or from `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` when running inside the cluster.

**Apiserver endpoint**

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

# Development

## Testing

### Unittests

Simply run `make test-unit`

### End-to-End

This project provides easy to use e2e testing using Hetzner cloud. To run the e2e tests
locally, the following steps are required:

* Populate the environment variable `HZ_E2E_TOKEN` with a valid Hetzner cloud token
* Run `make e2e-cluster` to get a simple kubeadm cluster on Hetzner
* Run `hack/run-machine-controller.sh` to locally run the machine-controller for your freshly created cluster

If you want to use an existing cluster to test against, you can simply set the `KUBECONFIG` environment variable.
In this case, first make sure that a kubeconfig created by `make e2e-cluster` at `$(go env GOPATH)/src/github.com/kubermatic/machine-controller/.kubeconfig`
doesn't exist, since the tests will default to this hardcoded path and only use the env var as fallback.

Now you can either

* Run the tests for all providers via
  `go test -race -tags=e2e -parallel 240 -v -timeout 30m  ./test/e2e/... -identifier $USER`
* Check `test/e2e/provisioning/all_e2e_test.go` for the available tests, then run only a specific one via
  `go test -race -tags=e2e -parallel 24 -v -timeout 20m  ./test/e2e/... -identifier $USER -run $TESTNAME`

__Note:__ All e2e tests require corresponding credentials to be present, check
 [`test/e2e/provisioning/all_e2e_test.go`](test/e2e/provisioning/all_e2e_test.go) for details

__Note:__ After finishing testing, please clean up after yourself:

* Execute `./test/tools/integration/cleanup_machines.sh` while the machine-controller is still running
* Execute `make e2e-destroy` to clean up the test control plane

You can also insert your ssh key into the created instances by editing the manifests in
[`test/e2e/provisioning/testdata/`](test/e2e/provisioning/testdata)

# Troubleshooting

If you encounter issues [file an issue][1] or talk to us on the [#kubermatic channel][2] on the [Kubermatic Slack][3].

# Contributing

Thanks for taking the time to join our community and start contributing!

### Before you start

* Please familiarize yourself with the [Code of Conduct][4] before contributing.
* See [CONTRIBUTING.md][5] for instructions on the developer certificate of origin that we require.
* Read how [we're using ZenHub][6] for project and roadmap planning

### Pull requests

* We welcome pull requests. Feel free to dig through the [issues][1] and jump in.

# Changelog

See [the list of releases][7] to find out about feature changes.

[1]: https://github.com/kubermatic/machine-controller/issues
[2]: https://kubermatic.slack.com/messages/kubermatic
[3]: http://slack.kubermatic.io/
[4]: code-of-conduct.md
[5]: CONTRIBUTING.md
[6]: Zenhub.md
[7]: https://github.com/kubermatic/machine-controller/releases
