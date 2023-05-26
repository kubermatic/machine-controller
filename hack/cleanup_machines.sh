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
set -x

kubectl annotate --all=true --overwrite node kubermatic.io/skip-eviction=true
kubectl delete machinedeployment -n kube-system --all
kubectl delete machineset -n kube-system --all
kubectl delete machine -n kube-system --all
for try in {1..30}; do
  if kubectl get machine -n kube-system 2>&1 | grep -q 'No resources found.'; then exit 0; fi
  sleep 10s
done

echo "Error: couldn't delete all machines!"
exit 1
