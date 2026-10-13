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

cd "$(dirname "$0")/../.."
source hack/lib.sh

MANIFEST_FILE="${MANIFEST_FILE:-pkg/mirror/mirror-images.yaml}"
CRANE="${CRANE:-crane}"

command -v yq > /dev/null || {
  echo "ERROR: yq not installed"
  exit 1
}
command -v jq > /dev/null || {
  echo "ERROR: jq not installed"
  exit 1
}
command -v "$CRANE" > /dev/null || {
  echo "ERROR: crane not installed (set CRANE=/path/to/crane if it is not on PATH)"
  exit 1
}
[[ -f "$MANIFEST_FILE" ]] || {
  echo "ERROR: ${MANIFEST_FILE} not found"
  exit 1
}

# fail fast on malformed YAML; process substitution swallows yq errors otherwise.
yq 'true' "$MANIFEST_FILE" > /dev/null

# prevent CRANE_PLATFORM from resolving manifest-list digests to a
# platform-specific digest and causing a false mismatch.
unset CRANE_PLATFORM

echodate "Verifying upstream existence of images in ${MANIFEST_FILE}..."

pass=0
fail=0
failed_keys=()

# one compact JSON object per line; fields parsed by name, not position.
while IFS= read -r entry; do
  key=$(jq -r '.key' <<< "$entry")
  source=$(jq -r '.source' <<< "$entry")
  tag=$(jq -r '.tag' <<< "$entry")
  version=$(jq -r '.version' <<< "$entry")
  ref="${source}@${version}"
  tag_ref="${source}:${tag}"
  echodate "Checking ${key} -> ${ref}"

  # crane digest is cheaper than manifest: it only resolves the digest,
  # without downloading and printing the manifest body.
  resolved=$("$CRANE" digest "$ref" 2> /tmp/crane.err) || {
    err=$(cat /tmp/crane.err)
    echodate "  FAIL ${key}: crane digest failed for ${ref}: ${err}"
    fail=$((fail + 1))
    failed_keys+=("$key")
    continue
  }

  # anti-tamper: resolving by digest must return the same digest back.
  if [[ "$resolved" != "$version" ]]; then
    echodate "  FAIL ${key}: digest mismatch. expected=${version} got=${resolved}"
    fail=$((fail + 1))
    failed_keys+=("$key")
    continue
  fi

  # tag-to-digest anti-tamper: the human-readable tag we publish to the mirror
  # must still resolve to the pinned digest upstream. catches stale tag/version
  # pairs and upstream tag-remappings -- otherwise the mirror would publish
  # divergent content under a name implying upstream parity.
  resolved_tag=$("$CRANE" digest "$tag_ref" 2> /tmp/crane.err) || {
    err=$(cat /tmp/crane.err)
    echodate "  FAIL ${key}: crane digest failed for ${tag_ref}: ${err}"
    fail=$((fail + 1))
    failed_keys+=("$key")
    continue
  }

  if [[ "$resolved_tag" != "$version" ]]; then
    echodate "  FAIL ${key}: tag/digest mismatch. ${tag_ref} resolves to ${resolved_tag}, expected ${version}"
    fail=$((fail + 1))
    failed_keys+=("$key")
    continue
  fi

  echodate "  PASS ${key}"
  pass=$((pass + 1))
done < <(yq -o=json '.images' "$MANIFEST_FILE" | jq -c 'to_entries[] | {key: .key, source: .value.source, tag: .value.tag, version: .value.version}')

echodate "Summary: ${pass} passed, ${fail} failed"

# silent-success guard: if the loop never ran (empty .images, malformed YAML
# slipping past the early parse check, missing .images key), the script would
# otherwise exit 0 with nothing verified.
if [[ "$pass" -eq 0 && "$fail" -eq 0 ]]; then
  echo "ERROR: no entries verified -- check that ${MANIFEST_FILE} contains a non-empty .images map"
  exit 1
fi

if [[ "$fail" -gt 0 ]]; then
  echodate "Failed entries:"
  for k in "${failed_keys[@]}"; do
    echodate "  - ${k}"
  done
  exit 1
fi

exit 0
