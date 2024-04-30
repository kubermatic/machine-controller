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
OSM_REPO_URL="${OSM_REPO_URL:-https://github.com/kubermatic/operating-system-manager.git}"
OSM_REPO_TAG="${OSM_REPO_TAG:-main}"

# cert-manager is required by OSM for generating TLS Certificates
echodate "Installing cert-manager"
(
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.11.2/cert-manager.yaml
  # Wait for cert-manager to be ready
  kubectl -n cert-manager rollout status deploy/cert-manager
  kubectl -n cert-manager rollout status deploy/cert-manager-cainjector
  kubectl -n cert-manager rollout status deploy/cert-manager-webhook
)

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
  if [[ ${LC_JOB_NAME:-} == "pull-machine-controller-e2e-ubuntu-upgrade" ]]; then
    sed -i '/.*join-cluster-timeout=.*/d' examples/machine-controller.yaml
  fi
  sed -i -e 's/-worker-count=5/-worker-count=50/g' examples/machine-controller.yaml
  # This is required for running e2e tests in KIND
  url="-override-bootstrap-kubelet-apiserver=$MASTER_URL"
  sed -i "s;-node-csr-approver=true;$url;g" examples/machine-controller.yaml

  # e2e tests logs are primarily read by humans, if ever
  sed -i 's/log-format=json/log-format=console/g' examples/machine-controller.yaml

  kubectl apply -f examples/machine-controller.yaml
  touch machine-controller-deployed

  protokol --kubeconfig "$KUBECONFIG" --flat --output "$ARTIFACTS/logs" --namespace kube-system 'machine-controller-*' > /dev/null 2>&1 &
fi

OSM_TMP_DIR=/tmp/osm
echodate "Clone OSM respository"
(
  # Clone OSM repo
  mkdir -p $OSM_TMP_DIR
  echodate "Cloning cluster exposer"
  git clone --depth 1 --branch "${OSM_REPO_TAG}" "${OSM_REPO_URL}" $OSM_TMP_DIR
)

(
  OSM_TAG="$(git -C $OSM_TMP_DIR rev-parse HEAD)"
  echodate "Installing operating-system-manager with image: $OSM_TAG"

  # In release branches we'll have this pinned to a specific semver instead of latest.
  sed -i "s;:latest;:$OSM_TAG;g" examples/operating-system-manager.yaml

  # This is required for running e2e tests in KIND
  url="-override-bootstrap-kubelet-apiserver=$MASTER_URL"
  sed -i "s;-container-runtime=containerd;$url;g" examples/operating-system-manager.yaml
  sed -i -e 's/-worker-count=5/-worker-count=50/g' examples/operating-system-manager.yaml
  kubectl apply -f examples/operating-system-manager.yaml
)

protokol --kubeconfig "$KUBECONFIG" --flat --output "$ARTIFACTS/logs" --namespace kube-system 'operating-system-manager-*' > /dev/null 2>&1 &

sleep 10
retry 10 check_all_deployments_ready kube-system

echodate "Finished installing machine-controller"
