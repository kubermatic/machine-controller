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

set -euo pipefail

cd $(dirname $0)/../..
source hack/lib.sh

if provider_disabled "${CLOUD_PROVIDER:-}"; then
  exit 0
fi

function cleanup {
  set +e

  # Clean up machines and services
  echo "Cleaning up machines and services..."
  ./hack/ci/cleanup.sh

  # Kill background port forward if it's there
  pkill ssh || true
}
trap cleanup EXIT

export GIT_HEAD_HASH="$(git rev-parse HEAD)"
export MC_VERSION="${GIT_HEAD_HASH}"
export OPERATING_SYSTEM_MANAGER="${OPERATING_SYSTEM_MANAGER:-true}"

TEST_NAME="Pre-warm Go build cache"
echodate "Attempting to pre-warm Go build cache"

beforeGocache=$(nowms)
make download-gocache
pushElapsed gocache_download_duration_milliseconds $beforeGocache

beforeBuild=$(nowms)
echodate "Building machine-controller and webhook..."
make all
pushElapsed binary_build_duration_milliseconds $beforeBuild

# Copy userdata plugins.
echodate "Copying machine-controller plugins..."
cp machine-controller-userdata-* /usr/local/bin
ls -l /usr/local/bin

# Install genisoimage, this is required for generating user-data for vSphere
if [[ "${JOB_NAME:-}" = *"pull-machine-controller-e2e-vsphere"* ]]; then
  echo "Installing genisoimage..."
  apt install -y genisoimage
fi

echodate "Creating kind cluster"
source hack/ci/setup-kind-cluster.sh

echodate "Setting up machine-controller in kind on revision ${MC_VERSION}"

beforeMCSetup=$(nowms)

source hack/ci/setup-machine-controller-in-kind.sh
pushElapsed kind_mc_setup_duration_milliseconds $beforeMCSetup

echo "Running e2e tests..."
EXTRA_ARGS=""
if [[ $# -gt 0 ]]; then
  EXTRA_ARGS="-run $1"
fi
go test -race -tags=e2e -parallel 240 -v -timeout 70m ./test/e2e/... -identifier=$BUILD_ID $EXTRA_ARGS

echo "Cleaning up machines and services..."
source hack/ci/cleanup.sh
