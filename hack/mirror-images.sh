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

set -eo pipefail

# Defaults; environment may override.
REGISTRY_HOST="${REGISTRY_HOST:-quay.io}"
REPOSITORY_PREFIX="${REPOSITORY_PREFIX:-kubermatic-mirror/images}"
MANIFEST_FILE="${MANIFEST_FILE:-pkg/mirror/mirror-images.yaml}"

set -u

# --- Usage -------------------------------------------------------------------
usage() {
  echo "Usage: $0 [image-key]"
  echo "  With no args, mirrors all images defined in ${MANIFEST_FILE}."
  echo "  With a key, mirrors only that one image."
  echo
  echo "Environment overrides: REGISTRY_HOST, REPOSITORY_PREFIX, MANIFEST_FILE,"
  echo "                       QUAY_IO_USERNAME, QUAY_IO_PASSWORD (skip Vault)."
  exit 1
}

# --- Vault login -------------------------------------------------------------
login_vault() {
  if [ -z "${VAULT_ADDR:-}" ]; then
    export VAULT_ADDR=https://vault.kubermatic.com/
  fi

  if [ -n "${VAULT_TOKEN:-}" ] || vault token lookup &> /dev/null; then
    return 0
  fi

  if [ -z "${VAULT_ROLE_ID:-}" ] || [ -z "${VAULT_SECRET_ID:-}" ]; then
    echo "Logging into Vault interactively using OIDC"
    vault login --method=oidc --path="${VAULT_OIDC_AUTH_PATH:-loodse}"
    return 0
  fi

  echo "Logging into Vault using prow CI credentials"
  local token
  token=$(vault write --format=json auth/approle/login "role_id=$VAULT_ROLE_ID" "secret_id=$VAULT_SECRET_ID" | jq -r '.auth.client_token')

  export VAULT_TOKEN="$token"
}

# --- Registry login ----------------------------------------------------------
# Resolves push credentials from the secret store when not already provided
# via env. QUAY_IO_USERNAME/QUAY_IO_PASSWORD must NOT be pre-populated by the
# job spec with credentials scoped to a different organization -- doing so
# bypasses this lookup and causes pushes to fail with 403 UNAUTHORIZED.
login_registry() {
  if [ -z "${QUAY_IO_USERNAME:-}" ] || [ -z "${QUAY_IO_PASSWORD:-}" ]; then
    login_vault
    : "${QUAY_IO_USERNAME:=$(vault kv get -field=username dev/kubermatic-mirror-quay.io)}"
    : "${QUAY_IO_PASSWORD:=$(vault kv get -field=password dev/kubermatic-mirror-quay.io)}"
  fi

  crane auth login "${REGISTRY_HOST}" --username "${QUAY_IO_USERNAME}" --password-stdin <<< "${QUAY_IO_PASSWORD}"
}

# --- Existence check ---------------------------------------------------------
image_exists_in_registry() {
  local dst="$1"
  crane manifest "$dst" > /dev/null 2>&1
}

# --- Mirror one image --------------------------------------------------------
mirror_image() {
  local key="$1" source="$2" version="$3"
  local src="${source}@${version}"
  local dst="${REGISTRY_HOST}/${REPOSITORY_PREFIX}/${key}@${version}"

  echodate "Mirroring ${key}:"
  echodate "  source:      ${src}"
  echodate "  destination: ${dst}"

  if image_exists_in_registry "$dst"; then
    echodate "  -> already mirrored, skipping"
    return 0
  fi
  crane copy "$src" "$dst"
  echodate "  -> done"
}

# --- Read manifest -----------------------------------------------------------
read_manifest() {
  yq -r '.images | to_entries[] | [.key, .value.source, .value.version] | @tsv' "$MANIFEST_FILE"
}

# --- Main --------------------------------------------------------------------
main() {
  cd "$(dirname "$0")/.."
  source hack/lib.sh

  [[ -f "$MANIFEST_FILE" ]] || {
    echo "ERROR: ${MANIFEST_FILE} not found"
    exit 1
  }
  command -v yq > /dev/null || {
    echo "ERROR: yq not installed"
    exit 1
  }
  command -v crane > /dev/null || {
    echo "Installing crane..."
    go install github.com/google/go-containerregistry/cmd/crane@latest
    export PATH="${PATH}:$(go env GOPATH)/bin"
  }

  local only_key="${1:-}"
  login_registry

  while IFS=$'\t' read -r key source version; do
    if [[ -n "$only_key" && "$key" != "$only_key" ]]; then continue; fi
    mirror_image "$key" "$source" "$version"
  done < <(read_manifest)
}

# Sourceable guard -- matches kubermatic/hack/mirror-application-charts.sh.
# When sourced, expose functions without triggering login or mirroring.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
