 #### Image builder script

 The script `build.sh` automatically builds a custom OS image for a VSphere environment. The original image of a selected OS is enriched with Kubernetes binaries, as well as (in the future) other custom files.

 Currently supported operating systems:
  * RedHat CoreOS
  * CentOS 7
  * Debian 9

### Usage

`./build.sh --target-os coreos|centos7|debian9 [--release K8S-RELEASE]`

Parameters:
 * `--target-os` is mandatory and specifies the Linux distribution image to be built. Possible values:
   * `coreos`
   * `centos7`
   * `debian9`
 * `--release` specifies the Kubernetes release to be added to the image, e.g. `v1.10.2`. If not provided, the script will look up the latest stable release and use that.

### Output

The script will generate a VMDK disk image with the filename `TARGET_OS-output.vmdk`, e.g. `coreos-output.vmdk`.
