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

source hack/lib.sh

if [ -z "${KIND_CLUSTER_NAME:-}" ]; then
  echodate "KIND_CLUSTER_NAME must be set by calling setup-kind-cluster.sh first."
  exit 1
fi

export MC_VERSION="${MC_VERSION:-$(git rev-parse HEAD)}"
export OPERATING_SYSTEM_MANAGER="${OPERATING_SYSTEM_MANAGER:-true}"

# Build the Docker image for machine-controller
beforeDockerBuild=$(nowms)

echodate "Building machine-controller Docker image"
TEST_NAME="Build machine-controller Docker image"
IMAGE_NAME="quay.io/kubermatic/machine-controller:latest"
time retry 5 docker build -t "$IMAGE_NAME" .
time retry 5 kind load docker-image "$IMAGE_NAME" --name "$KIND_CLUSTER_NAME"

pushElapsed mc_docker_build_duration_milliseconds $beforeDockerBuild
echodate "Successfully built and loaded machine-controller image"

if [ ! -f machine-controller-deployed ]; then
  # The 10 minute window given by default for the node to appear is too short
  # when we upgrade the instance during the upgrade test
  if [[ ${LC_JOB_NAME:-} = "pull-machine-controller-e2e-ubuntu-upgrade" ]]; then
    sed -i '/.*join-cluster-timeout=.*/d' examples/machine-controller.yaml
  fi
  sed -i -e 's/-worker-count=5/-worker-count=50/g' examples/machine-controller.yaml
  # This is required for running e2e tests in KIND
  url="-override-bootstrap-kubelet-apiserver=$MASTER_URL"
  sed -i "s;-node-csr-approver=true;$url;g" examples/machine-controller.yaml

  # Ensure that we update `use-osm` flag if OSM is disabled
  if [[ "$OPERATING_SYSTEM_MANAGER" == "false" ]]; then
    sed -i "s;-use-osm=true;-use-osm=false;g" examples/machine-controller.yaml
  fi

  make deploy
  touch machine-controller-deployed
fi

if [[ "$OPERATING_SYSTEM_MANAGER" == "true" ]]; then
  # cert-manager is required by OSM for generating TLS Certificates
  echodate "Installing cert-manager"
  (
    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.7.1/cert-manager.yaml
    # Wait for cert-manager to be ready
    kubectl -n cert-manager rollout status deploy/cert-manager
    kubectl -n cert-manager rollout status deploy/cert-manager-cainjector
    kubectl -n cert-manager rollout status deploy/cert-manager-webhook
  )

  echodate "Installing operating-system-manager"
  (
    # This is required for running e2e tests in KIND
    url="-override-bootstrap-kubelet-apiserver=$MASTER_URL"
    sed -i "s;-container-runtime=containerd;$url;g" examples/operating-system-manager.yaml
    sed -i -e 's/-worker-count=5/-worker-count=50/g' examples/operating-system-manager.yaml
    kubectl apply -f examples/operating-system-manager.yaml
  )
fi

sleep 10
retry 10 check_all_deployments_ready kube-system

echodate "Finished installing machine-controller"
