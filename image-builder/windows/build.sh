#!/bin/bash
export qemu_iso_path="https://s3-bucket-path-to/SW_DVD9_Win_Server_STD_CORE_20H2.3_64Bit_English_SAC_DC_STD_MLF_-2_X22-50889.ISO"
export qemu_iso_checksum="sha256:c82bca6d1188c2ec60de46822a1a04ebbdd4062ccbdf50a73161ad9746516e33"
export qemu_vm_name="Win Server STD CORE 20H2.3 English"
export win_admin_username="Administrator"
export win_admin_password="pGWd9B0dMZD0XkwumBnJ"
export win_accept_eula="false"
export win_register_username="User Name"
export win_register_organization="Organization Name"
export win_timezone="UTC"
# using KMS client setup key as placeholder (https://docs.microsoft.com/en-us/windows-server/get-started/kmsclientkeys)
export win_product_key="N2KJX-J94YW-TQVFB-DG9YT-724CC"

mkdir -p output
cat Autounattend-env.xml | envsubst > Autounattend.xml
cat scripts/Set-ProductKey-env.ps1 | envsubst > scripts/Set-ProductKey.ps1

# Download qemu drivers
mkdir -p driver/virtio-win
# TODO: Use WHQL Signed Drivers if user has a RHEL subscription
wget "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/latest-virtio/virtio-win.iso"
7z x -o"driver/virtio-win" virtio-win.iso
rm -f virtio-win.iso
mkdir -p driver/qemu/NetKVM
mkdir -p driver/qemu/vioscsi
mkdir -p driver/qemu/viostor
mv driver/virtio-win/NetKVM/2k19/amd64/*.{"cat","dll","inf","sys"} driver/qemu/NetKVM/
mv driver/virtio-win/vioscsi/2k19/amd64/*.{"cat","inf","sys"} driver/qemu/vioscsi/
mv driver/virtio-win/viostor/2k19/amd64/*.{"cat","inf","sys"} driver/qemu/viostor/
rm -rf driver/virtio-win

packer build -timestamp-ui Win_Server_STD_CORE_20H2.3_64Bit_English.json
