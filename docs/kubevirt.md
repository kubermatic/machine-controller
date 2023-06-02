# Kubevirt

In order to use the machine-controller to create machines using [Kubevirt](https://kubevirt.io)
you must first install the latter. We provide a manifest for this, simply run `kubectl apply -f examples/kubevirt-operator-0.19.0.yaml`.
We strongly recommend installing a version which is equal or higher than `0.19.0`. Machine Controller also uses the KubeVirt CDI which can be found
under `examples/cdi-operator.yaml` to provision storage. It is important to have a basic understanding of Kubernetes storage. For more
information regarding which types of storage can be used please refer to [KubeVirt documentation](https://github.com/kubevirt/containerized-data-importer/blob/main/doc/basic_pv_pvc_dv.md).


Afterwards, you can use the provided `examples/kubevirt-machinedeployment.yaml` as base. There
are some things you need to keep in mind:

* The machine-controller will create `VMIs` that have the same name as the underlying `machine`. To
avoid collisions, use one namespace per cluster that runs the `machine-controller`
* EvictionStratey of `VMIs` is set to external, so VMI eviction needs to handled properly by a custom external controller or manual action
* Service CIDR range: The CIDR ranges of the cluster that runs Kubevirt and the cluster that hosts the machine-controller must not overlap,
otherwise routing of services that run in the kubevirt cluster won't work anymore. This is especially important for the DNS ClusterIP.
* `clusterName` is used to [label VMs](https://github.com/kubevirt/cloud-provider-kubevirt#prerequisites) for LoadBalancer selection

## Serving Supported Images

For KubeVirt clusters, we use Containerized Data Importer (CDI), which is is a utility to import, upload and clone
Virtual Machine images for use with KubeVirt. At a high level, a persistent volume claim (PVC), which defines VM-suitable
storage via a storage class, is created.

The Containerized Data Importer is capable of performing certain functions that streamline its use with KubeVirt. It automatically
decompresses gzip and xz files, and un-tar’s tar archives. Also, qcow2 images are converted into the raw format which is required by KubeVirt,
resulting in the final file being a simple .img file.

Supported file formats are:

- Tar archive
- Gzip compressed file
- XZ compressed file
- Raw image data
- ISO image data
- Qemu qcow2 image data

KubeVirt reads those images from an http endpoint which is passed to the `MachineDeployment` spec. The field that should be used
for to import those images is `sourceURL`.
