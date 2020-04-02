# RedHat Enterprise Linux

Cloud providers which are listed below, support using rhel as an operating system option: 

- AWS 
- Azure
- GCE
- KubeVirt
- Openstack
- vSphere

####  AWS:
For amazon web service cloud provider, one the user applies for Gold Accesses Images using their RedHat Customer Portal, their request 
will be reviewed and validated. Afterwards, new images will be added to the aws account under EC2-> Images-> AMIs-> Private Images.
Once the images are available in the aws account, the image id for rhel(at the moment we only support 8.x.x versions) should be then 
added to the `MachineDeployment` spec to the field `ami`.

####  Azure
Same for aws, RedHat supports Azure and provides images for it. Once those images are available, the field `imageID` in the 
`MachineDeployment` spec should be updated with this image. 

**Note:** 
Azure rhel images starting from 7.6.x don't support cloud-init as their documentation states [here](https://docs.microsoft.com/en-us/azure/virtual-machines/linux/using-cloud-init#rhel).
Thus, custom images can be used with a cloud-init pre-installed to solve this issue. Follow this [documentation](https://docs.microsoft.com/en-us/azure/virtual-machines/linux/cloudinit-prepare-custom-image)
to prepare an image with cloud-init support.
 
####  GCE
RedHat also provides Gold Access Image for GCE and those can be fetched just like aws and azure. The image id should be added to 
the `MachineDeployment` spec in the field `customImage`. 

**Note:** 
Same as for Azure, rhel images in GCE don't support cloud-init. Thus, custom images can be used with a cloud-init pre-installed
to solve this issue. Follow this [documentation](https://cloud.google.com/compute/docs/images/export-image) to upload custom rhel
images in order to use it for running rhel instances.

####  KubeVirt
In order to create machines which run rhel as an operating system in KubeVirt cloud provider, the image should be available and fetched
via an endpoint. This endpoint should be then added to the `Machinedeployment` spec in the field `sourceURL`. For more information about 
the supported images please refer to this documentation from KubeVirt CDI [here](https://kubevirt.io/2018/containerized-data-importer.html)

####  Openstack
Once RHEL images(e.g: Red Hat Enterprise Linux 8.x KVM Guest Image) is uploaded to openstack, the image name should be used in 
the `MachineDeployment` spec in the filed `image`.

####  vSphere
To rhel os for vSphere instance, a template for the rhel machine should be created or a clone from a rhel machine. To upload rhel 
image to vSphere, follow these steps to create instances from a cloned machine:

- Download Red Hat Enterprise Linux 8.x KVM Guest Image from Red Hat Customer Portal.
- The image has the format `qcow2` thus should be converted to `vmdk` by running the command: `qemu-img convert -f qcow2 rhel.qcow2 -O vmdk newRHEL.vmdk`
- Run `vmkfstools -i myImage.vmdk outputName.vmdk -d thin To ensure that the` to ensure that, the `vmdk` is ESXi compatible.
- Upload the image to vSphere Datastore. Preferably use [`govc`](https://github.com/vmware/govmomi/blob/master/govc/USAGE.md#datastoreupload)
- Once the image is uploaded create a new instance using that image. During the machine creation process, at the `Customize Hardware`
step, press on ADD NEW DEVICE and select Existing Hard Disk. 
- In the Existing Hard Disk wizard select the rhel image file and then create the instance.
- Once the instance and everything is working as expected, power it off. 
- Use the instance name to clone rhel machine by updating the `MachineDeployment` spec and the name in the `templateVMName` field.
