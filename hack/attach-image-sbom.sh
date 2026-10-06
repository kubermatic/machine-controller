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

set -euo pipefail
cd $(dirname "$0")/..
source hack/lib.sh # for echodate / retry

imageRef="${1:?image reference is required}" # e.g. quay.io/kubermatic/osm:v1.12.0
repo="${imageRef%:*}"                        # quay.io/kubermatic/osm
mkdir -p _dist/images

# SBOM + attach for ONE concrete image digest
sbom_for_digest() {
  local digest="$1" # sha256:...
  local suffix="$2" # "" for single-arch, "-amd64"/"-arm64v8" for multi-arch
  local ref="$repo@$digest"

  # step 2: already has an SBOM? then do nothing
  if oras discover --format json "$ref" |
    jq -e '[.. | objects | select(.artifactType? == "application/spdx+json")] | length > 0' > /dev/null; then
    echodate "SBOM already attached to $ref, skipping."
    return
  fi

  # step 3: generate
  local sbomFile="_dist/images/$(basename "$repo")$suffix.sbom.spdx.json"
  echodate "Generating SBOM for $ref..."
  syft "registry:$ref" -o "spdx-json=$sbomFile"

  # step 4: attach next to the image in the registry
  echodate "Attaching $(basename "$sbomFile") to $ref..."
  (cd "$(dirname "$sbomFile")" && retry 3 oras attach --artifact-type application/spdx+json \
    "$ref" "$(basename "$sbomFile"):application/spdx+json")
}

# step 1: what is behind the tag?
manifest="$(oras manifest fetch "$imageRef")"

if jq -e '.manifests' <<< "$manifest" > /dev/null; then
  # multi-arch: a list of per-arch images; skip buildx's "unknown/unknown" attestation entries
  jq -r '.manifests[] | select(.platform.os != "unknown")
         | "\(.digest) \(.platform.architecture)\(.platform.variant // "")"' <<< "$manifest" |
    while read -r digest arch; do
      sbom_for_digest "$digest" "-$arch"
    done
else
  # single-arch: just one image
  sbom_for_digest "$(oras manifest fetch --descriptor "$imageRef" | jq -er '.digest')" ""
fi
