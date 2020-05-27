## Provision
The `provision` will identify the operating system and execute a set of
provisioning steps. It helps to provision a host exacly the same way as
machine-controller would do.

## Use-cases
- Prepare VM images for machine-controller
- (In future) Join host to existing Kubernetes cluster using bootstrap token

## Supported operating systems
- [x] CentOS 7
- [ ] CentOS 8
- [ ] Flatcar Flatcar Container Linux
- [x] Red Hat Enterprise Linux 8
- [x] SUSE Linux Enterprise Server
- [x] Ubuntu 18.04
- [ ] Ubuntu 20.04

## CLI / flags
TBD

## Provisioning steps
- Install required packages (apt / yum / dnf)
- Configure required kernel parameter (Like ip forwarding, etc.)
- Configure required kernel modules
- Disable swap
- Download & install the CNI plugins
- Download & install container runtime
- Download & install Kubelet

## Outputs
- `--output=direct` (default)  
  directy write files in their respected places and execute `/opt/bin/setup.sh`
  as entrypoint
- `--output=userdata`  
  output cloud-init userdata as is
- `--output=shell`  
  output shell script intended for remote execution over SSH, that will
  provision the system like `--output=direct` do.
- `--output=packer`  
  generate `package.json` file to be consumed by [Packer][packer]

[packer]: https://www.packer.io
