# Proxmox Virtual Environment

## State of the implementation

Support for Proxmox as a provider in the machine-controller is currently just a technical demo. It
is possible to create MachineDeployments using manually created VM templates as demonstrated below.
In this example the VM template is using local storage, which is why this template can only be
cloned on the same node it is located at.

## Prerequisites

### Authentication

For authentication the following data is needed:

- `user_id` is expected to be in the form `USER@REALM!TOKENID`
- `token` is just the UUID you get when initially creating the token

See also:
* https://pve.proxmox.com/wiki/User_Management#pveum_tokens
* https://pve.proxmox.com/wiki/Proxmox_VE_API#API_Tokens

#### User Privileges

For the provider to properly function the user needs an API token with the following privileges:

* `Datastore.AllocateSpace`
* `Pool.Allocate`
* `Pool.Audit`
* `VM.Allocate`
* `VM.Audit`
* `VM.Clone`
* `VM.Config.CDROM`
* `VM.Config.CPU`
* `VM.Config.Cloudinit`
* `VM.Config.Disk`
* `VM.Config.HWType`
* `VM.Config.Memory`
* `VM.Config.Network`
* `VM.Config.Options`
* `VM.Monitor`
* `Sys.Audit`
* `Sys.Console`

### Cloud-Init enabled VM Templates

Although it is possible to upload Cloud-Init images in Proxmox VE and create VM disks directly from
these images via CLI tools on the nodes directly, there is no API endpoint yet to provide this
functionality externally. That's why the `proxmox` provider assumes there are VM templates in place
to clone new machines from.

Proxmox recommends using either ready-to-use Cloud-Init images provided by many Linux distributions
(mostly designed for OpenStack) or to prepare the images yourself as you have full control over
what's in these images.

For VM templates to be available on all nodes, they need to be added to the `ha-manager`.

Example for creating a VM template:
```bash
# Download the cloud-image.
wget https://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-amd64.img
INSTANCE_ID=9000
# Create the VM that will be turned into the template.
qm create $INSTANCE_ID -name ubuntu-18.04-LTS
# Import the downloaded cloud-image as disk.
qm importdisk $INSTANCE_ID bionic-server-cloudimg-amd64.img local-lvm
# Set the imported Disk as SCSI drive.
qm set $INSTANCE_ID -scsihw virtio-scsi-pci -scsi0 local-lvm:vm-$INSTANCE_ID-disk-0
# Create the cloud-init drive where the user-data is read from.
qm set $INSTANCE_ID -ide2 local-lvm:cloudinit
# Boot from the imported disk.
qm set $INSTANCE_ID -boot c -bootdisk scsi0
# Set a serial console for better proxmox access.
qm set $INSTANCE_ID -serial0 socket -vga serial0
# Setup bridged network for the VM.
qm set $INSTANCE_ID -net0 virtio,bridge=vmbr0
# Enable QEMU Agent support for this VM (mandatory).
qm set $INSTANCE_ID -agent 1
# Convert VM to tempate.
qm template $INSTANCE_ID
# Make VM template available on any node, not just were it was created.
ha-manager add vm:$INSTANCE_ID -state stopped
```

### Cloud-Init user-data

Proxmox currently does not support the upload of "snippets" via API, but these snippets are used for
cloud-init user-data which are required for the machine-controller to function. This provider
implementation needs to copy the generated user-data yaml file to every proxmox node where a VM is
created or migrated to.

* A storage needs to be enabled for content `snippets` (e.g. `local`)
* SSH private key of a user that exists on all nodes and has write permission to the path where
snippets are stored (e.g. `/var/lib/vz/snippets`)
