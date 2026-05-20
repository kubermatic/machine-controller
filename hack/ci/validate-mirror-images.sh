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

MANIFEST_FILE="${MANIFEST_FILE:-hack/mirror-images.yaml}"
TEMPLATE_FILE="${TEMPLATE_FILE:-pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/client/template.go}"
MIRROR_PREFIX="${MIRROR_PREFIX:-quay.io/kubermatic-mirror/images}"
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

# template.go must reference only digests that are declared in the manifest,
# and every manifest entry must be referenced somewhere in template.go.
# Without this, a digest bump in Go without a YAML bump (or vice-versa) would
# merge silently: the postsubmit would not mirror the new digest, and Tinkerbell
# provisioning would fail at runtime with `manifest unknown`.
[[ -f "$TEMPLATE_FILE" ]] || {
  echo "ERROR: ${TEMPLATE_FILE} not found"
  exit 1
}

echodate "Checking ${TEMPLATE_FILE} digests are in sync with ${MANIFEST_FILE}..."

# digests declared in the manifest, one per line.
manifest_digests=$(yq -r '.images[].version' "$MANIFEST_FILE" | sort -u)

# digests referenced in template.go. Assumes each reference is a single-line
# string literal of the form ${MIRROR_PREFIX}/<key>@sha256:<hex>; if template.go
# is ever refactored to build these references with fmt.Sprintf or string
# concatenation, this grep stops matching and the empty-result branch below
# fails closed (exit 1) rather than silently passing.
# The `|| true` is scoped to grep alone so a no-match exit (1) does not abort
# under `set -e`; sort and awk failures still propagate.
template_digests=$({ grep -oE "${MIRROR_PREFIX}/[A-Za-z0-9._/-]+@sha256:[a-f0-9]{64}" "$TEMPLATE_FILE" || true; } |
  awk -F@ '{print $NF}' | sort -u)

if [[ -z "$template_digests" ]]; then
  echo "ERROR: no ${MIRROR_PREFIX}/...@sha256:... references found in ${TEMPLATE_FILE}"
  echo "  expected the Tinkerbell action images to be pinned to the mirror."
  exit 1
fi

unmirrored=$(comm -23 <(echo "$template_digests") <(echo "$manifest_digests"))
if [[ -n "$unmirrored" ]]; then
  echo "ERROR: ${TEMPLATE_FILE} references digests missing from ${MANIFEST_FILE}:"
  echo "$unmirrored" | sed 's/^/  - /'
  echo
  echo "Add an entry to ${MANIFEST_FILE} for each digest, or revert the change in ${TEMPLATE_FILE}."
  echo "Without a manifest entry, the postsubmit mirror job will not push these images."
  exit 1
fi

unused=$(comm -13 <(echo "$template_digests") <(echo "$manifest_digests"))
if [[ -n "$unused" ]]; then
  echo "ERROR: ${MANIFEST_FILE} entries not referenced by ${TEMPLATE_FILE}:"
  echo "$unused" | sed 's/^/  - /'
  echo
  echo "Remove the unused entries, or update ${TEMPLATE_FILE} to reference them."
  exit 1
fi

echodate "${TEMPLATE_FILE} and ${MANIFEST_FILE} are in sync."
