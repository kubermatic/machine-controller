#!/usr/bin/env bash

set -eu
set -o pipefail

SCRIPT_DIR="$(realpath "$(dirname "${BASH_SOURCE[0]}")")"
K8S_RELEASE=""
TARGET_OS=""

usage() {
  echo -e "usage:"
  echo -e "\t$0 --target-os coreos|centos7|debian9 [--release K8S-RELEASE]"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --release)
      K8S_RELEASE="$2"
      shift
      ;;
    --target-os)
      if [[ -z "$2" ]]; then
        echo "You must specify target OS. Currently 'coreos' and 'centos7' are supported."
        exit 1
      fi
      TARGET_OS="$2"
      shift
      ;;
    *)
      echo "Unknown parameter \"$1\""
      usage
      exit 1
      ;;
  esac
  shift
done

if [[ -z "$TARGET_OS" ]]; then
  usage
  exit 1
fi

if ! which guestmount &>/dev/null; then
  echo "guestmount is not available. On Ubuntu, you need to install libguestfs-tools"
  exit 1
fi

if ! which qemu-img &>/dev/null; then
  echo "qemu-img is not available. On Ubuntu, you need to install qemu-utils"
  exit 1
fi

if ! which gpg2 &>/dev/null; then
  echo "gpg2 is not available. On Ubuntu, you need to install gnupg2"
  exit 1
fi

# if no K8S version has was specified on the command line, get the latest stable
if [[ -z "$K8S_RELEASE" ]]; then
  K8S_RELEASE="$(curl -sSL https://dl.k8s.io/release/stable.txt)"
fi

TEMPDIR="$(mktemp -d)"
TARGETFS="$TEMPDIR/targetfs"
mkdir -p "$TARGETFS" "$SCRIPT_DIR/downloads"
# on failure unmount target filesystem (if mounted) and delte the temporary directory
trap "sudo mountpoint --quiet $TARGETFS && sudo umount $TARGETFS; rm -rf $TEMPDIR" EXIT SIGINT

get_coreos_image() {
  echo " * Downloading vanilla CoreOS image."
  wget https://stable.release.core-os.net/amd64-usr/current/coreos_production_vmware_image.vmdk.bz2{,.DIGESTS.asc} -P "$TEMPDIR"

  echo " * Verifying GPG signature"
  gpg2 --quiet --import "$SCRIPT_DIR/coreos_signing_key.asc"
  gpg2 "$TEMPDIR/coreos_production_vmware_image.vmdk.bz2.DIGESTS.asc"

  echo " * Verifying SHA512 digest"
  EXPECTED_SHA512="$(grep 'coreos_production_vmware_image.vmdk.bz2$' < "$TEMPDIR/coreos_production_vmware_image.vmdk.bz2.DIGESTS" | grep -P '([a-f0-9]){128}' | cut -f1 -d ' ')"
  CALCULATED_SHA512="$(sha512sum "$TEMPDIR/coreos_production_vmware_image.vmdk.bz2" | cut -f1 -d ' ')"
  if [[ "$CALCULATED_SHA512" != "$EXPECTED_SHA512" ]]; then
    echo " * SHA512 digest verification failed. '$CALCULATED_SHA512' != '$EXPECTED_SHA512'"
    exit 1
  fi

  echo " * Decompressing"
  bunzip2 --keep "$TEMPDIR/coreos_production_vmware_image.vmdk.bz2"
  mv "$TEMPDIR/coreos_production_vmware_image.vmdk" "$SCRIPT_DIR/downloads/coreos_production_vmware_image.original.vmdk"
}

get_centos7_image() {
  CENTOS7_BUILD="1802"
  echo " * Downloading vanilla CentOS image."
  wget "https://cloud.centos.org/centos/7/images/CentOS-7-x86_64-GenericCloud-$CENTOS7_BUILD.qcow2.xz" -P "$TEMPDIR"

  echo " * Verifying GPG signature"
  wget --quiet "https://cloud.centos.org/centos/7/images/sha256sum.txt.asc" -O "$TEMPDIR/centos7-sha256sum.txt.asc"
  gpg2 --quiet --import "$SCRIPT_DIR/RPM-GPG-KEY-CentOS-7"
  gpg2 "$TEMPDIR/centos7-sha256sum.txt.asc"

  echo " * Verifying SHA256 digest"
  EXPECTED_SHA256="$(grep "CentOS-7-x86_64-GenericCloud-$CENTOS7_BUILD.qcow2.xz$" < "$TEMPDIR/centos7-sha256sum.txt" | cut -f1 -d ' ')"
  CALCULATED_SHA256="$(sha256sum "$TEMPDIR/CentOS-7-x86_64-GenericCloud-$CENTOS7_BUILD.qcow2.xz" | cut -f1 -d ' ')"
  if [[ "$CALCULATED_SHA256" != "$EXPECTED_SHA256" ]]; then
    echo " * SHA256 digest verification failed. '$CALCULATED_SHA256' != '$EXPECTED_SHA256'"
    exit 1
  fi

  echo " * Decompressing"
  unxz --keep "$TEMPDIR/CentOS-7-x86_64-GenericCloud-$CENTOS7_BUILD.qcow2.xz"
  mv "$TEMPDIR/CentOS-7-x86_64-GenericCloud-$CENTOS7_BUILD.qcow2" "$SCRIPT_DIR/downloads/CentOS-7-x86_64-GenericCloud.qcow2"
}

get_debian9_image() {
  DEBIAN_CD_SIGNING_KEY_FINGERPRINT="DF9B9C49EAA9298432589D76DA87E80D6294BE9B"

  echo " * Downloading vanilla Debian image."
  wget "https://cdimage.debian.org/cdimage/openstack/current-9/debian-9-openstack-amd64.qcow2" -P "$TEMPDIR"

  echo " * Verifying GPG signature"
  wget --quiet "https://cdimage.debian.org/cdimage/openstack/current-9/SHA512SUMS" -O "$TEMPDIR/Debian-SHA512SUMS"
  wget --quiet "https://cdimage.debian.org/cdimage/openstack/current-9/SHA512SUMS.sign" -O "$TEMPDIR/Debian-SHA512SUMS.sign"
  gpg2 --quiet --recv-keys "$DEBIAN_CD_SIGNING_KEY_FINGERPRINT"
  gpg2 --quiet --verify "$TEMPDIR/Debian-SHA512SUMS.sign"

  echo " * Verifying SHA512 digest"
  EXPECTED_SHA512="$(grep 'debian-9-openstack-amd64.qcow2$' < "$TEMPDIR/Debian-SHA512SUMS" | cut -f1 -d ' ')"
  CALCULATED_SHA512="$(sha512sum "$TEMPDIR/debian-9-openstack-amd64.qcow2" | cut -f1 -d ' ')"
  if [[ "$CALCULATED_SHA512" != "$EXPECTED_SHA512" ]]; then
    echo " * SHA512 digest verification failed. '$CALCULATED_SHA512' != '$EXPECTED_SHA512'"
    exit 1
  fi

  echo " * Finalizing"
  mv "$TEMPDIR/debian-9-openstack-amd64.qcow2" "$SCRIPT_DIR/downloads/debian-9-openstack-amd64.qcow2"
}

case $TARGET_OS in
  coreos)
    CLEAN_IMAGE="$SCRIPT_DIR/downloads/coreos_production_vmware_image.original.vmdk"
    ROOTFS_PARTITION="/dev/sda9"
    if [[ ! -f "$CLEAN_IMAGE" ]]; then
      get_coreos_image
    fi
    ;;
  centos7)
  CLEAN_IMAGE="$SCRIPT_DIR/downloads/CentOS-7-x86_64-GenericCloud.qcow2"
    ROOTFS_PARTITION="/dev/sda1"
    if [[ ! -f "$CLEAN_IMAGE" ]]; then
      get_centos7_image
    fi
    ;;
  debian9)
    CLEAN_IMAGE="$SCRIPT_DIR/downloads/debian-9-openstack-amd64.qcow2"
    ROOTFS_PARTITION="/dev/sda1"
    if [[ ! -f "$CLEAN_IMAGE" ]]; then
      get_debian9_image
    fi
    ;;
  *)
    usage
    exit 1
esac

echo " * Verifying/Downloading kubernetes"
./download_kubernetes.sh --release "$K8S_RELEASE"

echo " * Mouting the image"
cp "$CLEAN_IMAGE" "$TEMPDIR/work-in-progress-image"
sudo guestmount -a "$TEMPDIR/work-in-progress-image" -m "$ROOTFS_PARTITION" "$TARGETFS"

echo " * Copying kubernetes binaries"
sudo mkdir -p "$TARGETFS/opt/bin/"
sudo cp "$SCRIPT_DIR/downloads/kubeadm-$K8S_RELEASE" "$TARGETFS/opt/bin/kubeadm"
sudo cp "$SCRIPT_DIR/downloads/kubectl-$K8S_RELEASE" "$TARGETFS/opt/bin/kubectl"
sudo cp "$SCRIPT_DIR/downloads/kubelet-$K8S_RELEASE" "$TARGETFS/opt/bin/kubelet"

echo " * Finalizing"
sudo umount "$TARGETFS"
EXTENSION="${CLEAN_IMAGE##*.}"
if [[ "$EXTENSION" == "vmdk" ]]; then
  cp "$TEMPDIR/work-in-progress-image" "$SCRIPT_DIR/$TARGET_OS-output.vmdk"
else
  echo " * Converting to VMDK"
  qemu-img convert -O vmdk "$TEMPDIR/work-in-progress-image" "$SCRIPT_DIR/$TARGET_OS-output.vmdk"
fi

echo "$(realpath "$SCRIPT_DIR/$TARGET_OS-output.vmdk") ready."
