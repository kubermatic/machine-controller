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

set -e

make -C $(dirname $0)/.. machine-controller
$(dirname $0)/../machine-controller \
  -kubeconfig=$(dirname $0)/../.kubeconfig \
  -worker-count=50 \
  -logtostderr \
  -v=6 \
  -cluster-dns=172.16.0.10 \
  -enable-profiling \
  -internal-listen-address=0.0.0.0:8085
