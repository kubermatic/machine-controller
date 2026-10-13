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
# OCI distribution spec: tag must match [a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}.
TAG_RE='^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$'

echodate "Validating ${MANIFEST_FILE}..."

# Ensure yq is installed
command -v yq > /dev/null || {
  echo "ERROR: yq not installed."
  exit 1
}

# Ensure we are using mikefarah/yq (Go version), not kislyuk/yq (Python version)
if yq --version 2>&1 | grep -q "jq wrapper"; then
  echo "ERROR: Detected Python 'yq' (kislyuk/yq). This script requires the Go version of 'yq' (mikefarah/yq v4+)."
  exit 1
fi

[[ -f "$MANIFEST_FILE" ]] || {
  echo "ERROR: ${MANIFEST_FILE} not found"
  exit 1
}

# fail fast on malformed YAML; process substitution swallows yq errors otherwise.
yq 'true' "$MANIFEST_FILE" > /dev/null

fail_if() {
  local message="$1" offenders="$2"
  [[ -z "$offenders" ]] && return 0
  echo "ERROR: ${message}"
  echo "$offenders" | sed 's/^/  - /'
  exit 1
}

# .images must exist and have at least one entry.
count=$(yq '.images | length' "$MANIFEST_FILE")
if [[ "$count" == "0" || "$count" == "null" ]]; then
  echo "ERROR: ${MANIFEST_FILE} has no .images entries"
  exit 1
fi
echodate "  ${count} entries found"

# every entry needs source, tag, and version fields. MUST run before the
# regex checks below: yq's test() crashes on null values.
missing=$(yq -r '
  .images | to_entries[]
  | select(.value.source == null or .value.tag == null or .value.version == null)
  | .key
' "$MANIFEST_FILE")
fail_if "entries missing source, tag, or version:" "$missing"

# source is a bare registry path -- no tags or digests allowed.
bad_sources=$(yq -r '
  .images | to_entries[]
  | select(.value.source | test("[:@]"))
  | .key + " -> " + .value.source
' "$MANIFEST_FILE")
fail_if "source field must be a bare registry path (no tag, no digest):" "$bad_sources"

# version must be an explicit sha256 digest -- tags and "latest" are forbidden.
bad_versions=$(yq -r "
  .images | to_entries[]
  | select(.value.version | test(\"$SHA_RE\") | not)
  | .key + \" -> \" + .value.version
" "$MANIFEST_FILE")
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

# tag must match the OCI distribution tag regex.
bad_tags=$(yq -r "
  .images | to_entries[]
  | select(.value.tag | test(\"$TAG_RE\") | not)
  | .key + \" -> \" + .value.tag
" "$MANIFEST_FILE")
fail_if "tag must match [a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}. Found:" "$bad_tags"

echodate "All ${MANIFEST_FILE} entries use valid sha256 digests and tags."
