#!/usr/bin/env bash

set -eu
set -o pipefail

TEMPDIR="$(mktemp -d)"
trap "rm -rf $TEMPDIR" EXIT SIGINT

SCRIPT_DIR="$(realpath "$(dirname "${BASH_SOURCE[0]}")")"
mkdir -p "$SCRIPT_DIR/downloads"

K8S_RELEASE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --release)
      K8S_RELEASE="$2"
      shift
    ;;
    *)
      echo "Unknown parameter \"$1\""
      exit 1
    ;;
  esac
  shift
done

if [[ -z "$K8S_RELEASE" ]]; then
  K8S_RELEASE="$(curl -sSL https://dl.k8s.io/release/stable.txt)"
  echo " * Latest stable version is $K8S_RELEASE"
else
  echo " * Using version $K8S_RELEASE"
fi

wget --quiet https://storage.googleapis.com/kubernetes-release/release/$K8S_RELEASE/bin/linux/amd64/{kubeadm,kubelet,kubectl}.sha1 -P "$TEMPDIR"

for util in kubeadm kubelet kubectl; do
  echo "   * $util"
  if [[ -x "$SCRIPT_DIR/downloads/$util-$K8S_RELEASE" ]]; then
    CALCULATED_SHA1="$(sha1sum "$SCRIPT_DIR/downloads/$util-$K8S_RELEASE" | cut -f1 -d ' ')"
    EXPECTED_SHA1="$(<"$TEMPDIR/$util.sha1")"
    if [[ "$CALCULATED_SHA1" != "$EXPECTED_SHA1" ]]; then
      echo " * SHA1 digest verification failed. $CALCULATED_SHA1 != $EXPECTED_SHA1"
      echo " * The downloaded $util is either corrupted or out of date. Check your downloads and remove manually to continue."
      exit 1
    fi
  else
    wget "https://storage.googleapis.com/kubernetes-release/release/$K8S_RELEASE/bin/linux/amd64/$util" -P "$TEMPDIR"

    CALCULATED_SHA1="$(sha1sum "$TEMPDIR/$util" | cut -f1 -d ' ')"
    EXPECTED_SHA1="$(<"$TEMPDIR/$util.sha1")"
    if [[ "$CALCULATED_SHA1" != "$EXPECTED_SHA1" ]]; then
      echo " * SHA1 digest verification failed. $CALCULATED_SHA1 != $EXPECTED_SHA1. Download failed."
      exit 1
    fi

    mv "$TEMPDIR/$util" "$SCRIPT_DIR/downloads/$util-$K8S_RELEASE"
    chmod +x "$SCRIPT_DIR/downloads/$util-$K8S_RELEASE"
  fi
done
