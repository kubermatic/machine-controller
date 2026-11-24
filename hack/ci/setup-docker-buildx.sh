#!/usr/bin/env bash

# Copyright 2026 The Machine Controller Authors.
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

# Prepares a (docker-in-docker) Docker daemon for multi-arch image builds:
# registers QEMU binfmt handlers for the non-native platforms and creates a
# buildx builder using the docker-container driver, which (unlike the default
# docker driver) can build and push multi-platform images.
# Requires a running Docker daemon and privileged mode.

set -euo pipefail

cd "$(dirname "$0")/../.."
source hack/lib.sh

BINFMT_IMAGE="${BINFMT_IMAGE:-docker.io/tonistiigi/binfmt:qemu-v10.2.3-68@sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0}"
BINFMT_ARCHS="${BINFMT_ARCHS:-arm64}"
BUILDER_NAME="${BUILDER_NAME:-multiarch}"

echodate "Registering QEMU binfmt handlers for: ${BINFMT_ARCHS}"
retry 5 docker run --privileged --rm "${BINFMT_IMAGE}" --install "${BINFMT_ARCHS}"

if docker buildx inspect "${BUILDER_NAME}" > /dev/null 2>&1; then
  echodate "Using existing buildx builder ${BUILDER_NAME}"
  docker buildx use "${BUILDER_NAME}"
else
  echodate "Creating buildx builder ${BUILDER_NAME}"
  docker buildx create --name "${BUILDER_NAME}" --driver docker-container --use
fi

retry 5 docker buildx inspect --bootstrap
