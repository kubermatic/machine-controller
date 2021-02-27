# VMware vSphere

## Supported versions

* 6.5
* 6.7

## Template VMs preparation

To use the machine-controller to create machines on VMWare vsphere, you must first
create a VM to be used as a template.

*Note that:*
`template VMs` in this document refers to regular VMs and not
[VM Templates][vm_templates] according to vSphere terminology.
The difference is quite subtle, but VM Templates are not supported yet by
`machine controller`.

### Create template VM from OVA

To see where to locate the OVAs go to the OS specific section.

#### WebUI procedure

1. Go into the vSphere WebUI, select your datacenter, right click onto it and choose "Deploy OVF Template"
2. Fill in the "URL" field with the appropriate url pointing to the `OVA` file
3. Click through the dialog until "Select storage"
4. Select the same storage you want to use for your machines
5. Select the same network you want to use for your machines
6. Leave everyhting in the "Customize Template" and "Ready to complete" dialog as it is
7. Wait until the VM got fully imported and the "Snapshots" => "Create Snapshot" button is not grayed out anymore

#### Command-line procedure

Prerequisites:

* [GOVC](https://github.com/vmware/govmomi/tree/master/govc): tested on version 0.22.1
* [jq](https://stedolan.github.io/jq/)

Procedure:

1. Download the `OVA` for the targeted OS.

    ```
    curl -sL "${OVA_URL}" -O .
    ```

2. Extract the specs from the `OVA`:

    ```
    govc import.spec $(basename "${OVA_URL}") | jq -r > options.json
    ```

3. Edit the `options.json` file with your text editor of choice.

    * Edit the `NetworkMapping` to point to the correct network.
    * Make sure that `PowerOn` is set to `false`.
    * Make sure that `MarkAsTemplate` is set to `false`.
    * Verify the other properties and customize according to your needs.
    e.g.

    ```json
    {
      "DiskProvisioning": "flat",
      "IPAllocationPolicy": "dhcpPolicy",
      "IPProtocol": "IPv4",
      "PropertyMapping": [
        {
          "Key": "guestinfo.hostname",
          "Value": ""
        },
        {
          "Key": "guestinfo.flatcar.config.data",
          "Value": ""
        },
        {
          "Key": "guestinfo.flatcar.config.url",
          "Value": ""
        },
        {
          "Key": "guestinfo.flatcar.config.data.encoding",
          "Value": ""
        },
        {
          "Key": "guestinfo.interface.0.name",
          "Value": ""
        },
        {
          "Key": "guestinfo.interface.0.mac",
          "Value": ""
        },
        {
          "Key": "guestinfo.interface.0.dhcp",
          "Value": "no"
        },
        {
          "Key": "guestinfo.interface.0.role",
          "Value": "public"
        },
        {
          "Key": "guestinfo.interface.0.ip.0.address",
          "Value": ""
        },
        {
          "Key": "guestinfo.interface.0.route.0.gateway",
          "Value": ""
        },
        {
          "Key": "guestinfo.interface.0.route.0.destination",
          "Value": ""
        },
        {
          "Key": "guestinfo.dns.server.0",
          "Value": ""
        },
        {
          "Key": "guestinfo.dns.server.1",
          "Value": ""
        }
      ],
      "NetworkMapping": [
        {
          "Name": "VM Network",
          "Network": "Loodse Default"
        }
      ],
      "MarkAsTemplate": false,
      "PowerOn": false,
      "InjectOvfEnv": false,
      "WaitForIP": false,
      "Name": null
    }
    ```

4. Create a VM from the `OVA`:

    ```
    govc import.ova -options=options.json $(basename "${OVA_URL}")
    ```

### Create template VM from qcow2

Prerequisites:

* vSphere (tested on version 6.7)
* GOVC (tested on version 0.22.1)
* qemu-img (tested on version 4.2.0)
* curl or wget

Procedure:

1. Download the guest image in qcow2 format end export an environment variable
   with the name of the file.

    ```
    # The URL below is just an example
    image_url="https://cloud.centos.org/centos/7/images/CentOS-7-x86_64-GenericCloud.qcow2"
    image_name="$(basename -- "${image_url}" | sed 's/.qcow2$//g')"
    curl -sL "${image_url}" -O .
    ```

2. Convert it to vmdk e.g.

    ```
    qemu-img convert -O vmdk -o subformat=streamOptimized "./${image_name}.qcow2" "${image_name}.vmdk"
    ```

3. Upload to vSphere using WebUI or GOVC:

    Make sure to replace the parameters on the command below with the correct
    values specific to yout vSphere environment.

    ```
    govc import.vmdk -dc=dc-1 -pool=/dc-1/host/cl-1/Resources -ds=ds-1 "./${image_name}.vmdk"
    ```

4. Inflate the created disk (see vmware [documentation][inflate_thin_virtual_disks] for more details)

    ```
    govc datastore.disk.inflate -dc dc-1 -ds ds-1 "${image_name}/${image_name}.vmdk"
    ```

5. Create a new virtual machine using that image with vSphere WebUI.
6. During the `Customize Hardware` step:
    1. Remove the disk present by default
    2. Click on `ADD NEW DEVICE`, select `Existing Hard Disk` and select the
       disk previously created.
7. The vm is ready to be used by the `MachineController` by referencing its name in the field `.spec.template.spec.providerSpec.value.cloudProviderSpec.templateVMName` of the `MachineDeployment`.

### OS images

Information about supported OS versions can be found [here](./operating-system.md#supported-os-versions).

#### Ubuntu

Ubuntu OVA template can be foud at <https://cloud-images.ubuntu.com/releases/18.04/release/ubuntu-18.04-server-cloudimg-amd64.ova>.

Follow [OVA](#create-template-vm-from-ova) template VM creation guide.

#### RHEL

Red Hat Enterprise Linux 8.x KVM Guest Image can be found at [Red Hat Customer Portal][rh_portal_rhel8].

Follow [qcow2](#create-template-vm-from-qcow2) template VM creation guide.

#### CentOS

CentOS 7 image can be found at the following link: <https://cloud.centos.org/centos/7/images/CentOS-7-x86_64-GenericCloud.qcow2r>.

Follow [qcow2](#create-template-vm-from-qcow2) template VM creation guide.

#### Windows

Windows Server 2019 images (testing only and registration required) can be found at the following link: <https://www.microsoft.com/evalcenter/evaluate-windows-server-2019>.
Windows Server Semi-Annual images can be found only within the VLSC or Visual Studio Subscription portal.
More information at the following link: <https://docs.microsoft.com/windows-server/get-started-19/servicing-channels-19>.

## Provider configuration

VSphere provider accepts the following configuration parameters:

```yaml
# Can also be set via the env var 'VSPHERE_USERNAME' on the machine-controller
username: '<< VSPHERE_USERNAME >>'
# Can also be set via the env var 'VSPHERE_ADDRESS' on the machine-controller
# example: 'https://your-vcenter:8443'. '/sdk' gets appended automatically
vsphereURL: '<< VSPHERE_ADDRESS >>'
# Can also be set via the env var 'VSPHERE_PASSWORD' on the machine-controller
password: "<< VSPHERE_PASSWORD >>"
# datacenter name
datacenter: datacenter1
# VM template name
templateVMName: ubuntu-template
# Optional. Sets the networks on the VM. If no network is specified, the template default will be used.
vmNetName: network1
# Optional
folder: folder1
# Optional: Force VMs to be provisoned to the specified resourcePool
# Default is to use the resourcePool of the template VM
# example: kubeone or /DC/host/Cluster01/Resources/kubeone
resourcePool: kubeone
cluster: cluster1
# either datastore or datastoreCluster have to be provided.
datastore: datastore1
datastoreCluster: datastore-cluster1
# Can also be set via the env var 'VSPHERE_ALLOW_INSECURE' on the machine-controller
allowInsecure: true
# instance resources
cpus: 2
memoryMB: 2048
# Optional: Resize the root disk to this size. Must be bigger than the existing size
# Default is to leave the disk at the same size as the template
diskSizeGB: 10
```

### Datastore and DatastoreCluster

A `Datastore` is the basic unit of storage abstraction in vSphere storage (more details [here][datastore]).

A `DatastoreCluster` (sometimes referred to as StoragePod) is a logical grouping of `Datastores`, it provides some resource management capabilities (more details [here][datastore_cluster]).

VSphere provider configuration in a `MachineDeployment` should specify either a `Datastore` or a `DatastoreCluster`. If both are specified or if one of the two is missing the `MachineDeployment` validation will fail.

*Note that*
the `datastore` or `datastoreCluster` specified in the `MachineDeployment` will be only used for the placement of VM and disk files related to the VMs provisioned by the `machine controller`. They do not influence the placement of persistent volumes used by PODs, that only depends on the cloud configuration given to the k8s cloud provider running in control plane.

[vm_templates]: https://docs.vmware.com/en/VMware-vSphere/6.7/com.vmware.vsphere.vm_admin.doc/GUID-F7BF0E6B-7C4F-4E46-8BBF-76229AEA7220.html?hWord=N4IghgNiBcIG4FsAEAXApggDhM6DOIAvkA
[datastore]: https://docs.vmware.com/en/VMware-vSphere/6.7/com.vmware.vsphere.storage.doc/GUID-3CC7078E-9C30-402C-B2E1-2542BEE67E8F.html
[datastore_cluster]: https://docs.vmware.com/en/VMware-vSphere/6.7/com.vmware.vsphere.resmgmt.doc/GUID-598DF695-107E-406B-9C95-0AF961FC227A.html
[inflate_thin_virtual_disks]: https://docs.vmware.com/en/VMware-vSphere/6.7/com.vmware.vsphere.storage.doc/GUID-C371B88F-C407-4A69-8F3B-FA877D6955F8.html
[rh_portal_rhel8]: https://access.redhat.com/downloads/content/479/ver=/rhel---8/8.1/x86_64/product-software
