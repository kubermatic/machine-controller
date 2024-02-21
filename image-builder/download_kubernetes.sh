#!/usr/bin/env bash

# Copyright 2019 The Machine Controller Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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

wget --quiet https://dl.k8s.io/$K8S_RELEASE/bin/linux/amd64/{kubeadm,kubelet,kubectl}.sha1 -P "$TEMPDIR"

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
    wget "https://dl.k8s.io/$K8S_RELEASE/bin/linux/amd64/$util" -P "$TEMPDIR"

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
