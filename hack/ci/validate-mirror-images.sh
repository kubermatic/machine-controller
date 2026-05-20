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
SHA_RE='^sha256:[a-f0-9]{64}$'

echodate "Validating ${MANIFEST_FILE}..."

command -v yq > /dev/null || {
  echo "ERROR: yq not installed"
  exit 1
}
[[ -f "$MANIFEST_FILE" ]] || {
  echo "ERROR: ${MANIFEST_FILE} not found"
  exit 1
}

# .images must exist and have at least one entry.
count=$(yq '.images | length' "$MANIFEST_FILE")
if [[ "$count" == "0" || "$count" == "null" ]]; then
  echo "ERROR: ${MANIFEST_FILE} has no .images entries"
  exit 1
fi
echodate "  ${count} entries found"

# every entry needs both source and version fields.
missing=$(yq -r '.images | to_entries[] | select(.value.source == null or .value.version == null) | .key' "$MANIFEST_FILE")
if [[ -n "$missing" ]]; then
  echo "ERROR: entries missing source or version:"
  echo "$missing" | sed 's/^/  - /'
  exit 1
fi

# source is a bare registry path -- no tags or digests allowed.
bad_sources=$(yq -r '.images | to_entries[] | [.key, .value.source] | @tsv' "$MANIFEST_FILE" |
  awk -F'\t' '$2 ~ /[:@]/ { print $1 " -> " $2 }')
if [[ -n "$bad_sources" ]]; then
  echo "ERROR: source field must be a bare registry path (no tag, no digest):"
  echo "$bad_sources" | sed 's/^/  - /'
  exit 1
fi

# version must be an explicit sha256 digest -- tags and "latest" are forbidden.
bad_versions=$(yq -r '.images | to_entries[] | [.key, .value.version] | @tsv' "$MANIFEST_FILE" |
  awk -F'\t' -v re="$SHA_RE" '$2 !~ re { print $1 " -> " $2 }')
if [[ -n "$bad_versions" ]]; then
  echo "ERROR: version must be sha256:<64-hex>. Found:"
  echo "$bad_versions" | sed 's/^/  - /'
  echo
  echo "Resolve the upstream tag to its digest with:"
  echo "  crane digest <source>:<tag>"
  echo
  echo "Tags (latest, semver, branch names) are forbidden by policy."
  exit 1
fi

echodate "All ${MANIFEST_FILE} entries use sha256 digests."
