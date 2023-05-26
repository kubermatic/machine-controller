# Provisioning

This command offers all required functionality to provision an host to join a Kubernetes cluster.

The following operating systems are supported

- Ubuntu 18.04
- CentOS 7
- Flatcar

## Requirements

- The cluster needs to use the bootstrap token authentication

## CLI

```bash
./provision \
    --kubelet-version="v1.13.1" \
    --cloud-provider="openstack" \
    --cloud-config="/etc/kubernetes/cloud-config" \
    --token="AAAAAAAAAAAAAAAA" \
    --ca-cert="/etc/kubernetes/ca.crt"
```

## Process

Nodes will boot with a cloud-init (Or Ignition) which writes required files & a shell script (called `setup.sh` here).

### cloud-init (Or ignition)

Parts which will be covered by cloud-init (or Ignition)

- Install SSH keys
- Configure hostname
- `ca.crt`
    The CA certificate which got used to issue the certificates of the API server serving certificates
- `cloud-config`
    A optional cloud-config used by the kubelet to interact with the cloud provider.
- `setup.sh`
    Is responsible for downloading the `provision` binary and to execute it.
    The download of the binary might also be done using built-in `cloud-init` (or Ignition) features

### Provision

The `provision` binary will identify the operating system and execute a set of provisioning steps.

The provisioning process gets separated into 2 phases:

- Base provisioning
  Install and configure all required dependencies
- Join
  Write & start the kubelet systemd unit

#### Base provisioning

The following steps belong into the base provisioning:

- Install required packages (apt & yum action)
- Configure required kernel parameter (Like ip forwarding, etc.)
- Configure required kernel modules
- Disable swap
- Download & install the CNI plugins
- Download & Install docker
- Download Kubelet
- Install health checks (Kubelet & Docker)

#### Join

This part will:

- Write & start the kubelet systemd unit

## Offline usage

The `provision` binary should also be usable for "prebaking" images, which then can be used for offline usage.

## Development process

To make sure the local development version of the `provision` command gets used for new machines created by the local running machine controller,
a new flag `--provision-source` must be introduced.
This flag will instruct the machine controller to download the `provision` binary from the specified location.

For simplicity the `/hack/run-machine-controller.sh` will be updated to include a step which will compile the `provoision` command & upload it to a gcs bucket.
