#!/usr/bin/env bash

# Copyright 2022 The Machine Controller Authors.
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

CNI_VERSION="${CNI_VERSION:-v0.8.7}"

cni_bin_dir=/opt/cni/bin
mkdir -p /etc/cni/net.d "$cni_bin_dir"
arch=${HOST_ARCH-}
if [ -z "$arch" ]; then
  case $(uname -m) in
  x86_64)
    arch="amd64"
    ;;
  aarch64)
    arch="arm64"
    ;;
  *)
    echo "unsupported CPU architecture, exiting"
    exit 1
    ;;
  esac
fi
cni_base_url="https://github.com/containernetworking/plugins/releases/download/$CNI_VERSION"
cni_filename="cni-plugins-linux-$arch-$CNI_VERSION.tgz"
curl -Lfo "$cni_bin_dir/$cni_filename" "$cni_base_url/$cni_filename"
cni_sum=$(curl -Lf "$cni_base_url/$cni_filename.sha256")
cd "$cni_bin_dir"
sha256sum -c <<< "$cni_sum"
tar xvf "$cni_filename"
rm -f "$cni_filename"
