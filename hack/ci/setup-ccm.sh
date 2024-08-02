#!/usr/bin/env bash

# Copyright 2024 The Machine Controller Authors.
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

echodate "Setting up $CLOUD_PROVIDER cloud-controller-manager…"

# kubectl taint node machine-controller-control-plane node.cloudprovider.kubernetes.io/uninitialized:PreferNoSchedule
kubectl delete node machine-controller-control-plane

case "$CLOUD_PROVIDER" in
aws)
  kubectl create secret generic cloud-credentials \
    --namespace kube-system \
    --from-literal "accessKeyId=$AWS_E2E_TESTS_KEY_ID" \
    --from-literal "secretAccessKey=$AWS_E2E_TESTS_SECRET"

  kubectl apply -f hack/ci/ccm/aws/
  ;;
esac
