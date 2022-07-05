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

set -euo pipefail

# Required for signal propagation to work so
# the cleanup trap gets executed when the script
# receives a SIGINT
set -o monitor

cd $(dirname $0)/../..

if [ -z "${GOCACHE_MINIO_ADDRESS:-}" ]; then
  echo "Fatal: env var GOCACHE_MINIO_ADDRESS unset"
  exit 1
fi

# The gocache needs a matching go version to work, so append that to the name
GO_VERSION="$(go version | awk '{ print $3 }' | sed 's/go//g')"

GOCACHE_DIR="$(mktemp -d)"
export GOCACHE="${GOCACHE_DIR}"
export GIT_HEAD_HASH="$(git rev-parse HEAD | tr -d '\n')"

# PULL_BASE_REF is the name of the current branch in case of a post-submit
# or the name of the base branch in case of a PR.
GIT_BRANCH="${PULL_BASE_REF:-master}"

# normalize branch name to prevent accidental directories being created
GIT_BRANCH="$(echo "$GIT_BRANCH" | sed 's#/#-#g')"

echo "Creating cache for revision ${GIT_HEAD_HASH} / Go ${GO_VERSION} ..."

echo "Building binaries"
make all

echo "Building tests"
make build-tests

echo "Creating gocache archive"
ARCHIVE_FILE="/tmp/${GIT_HEAD_HASH}.tar"
# No compression because that needs quite a bit of CPU
tar -C "$GOCACHE" -cf "$ARCHIVE_FILE" .

echo "Uploading gocache archive machine-controller/${GIT_BRANCH}/${GIT_HEAD_HASH}-${GO_VERSION}.tar"
curl \
  --fail \
  --upload-file "${ARCHIVE_FILE}" \
  --header "Content-Type: application/octet-stream" \
  "${GOCACHE_MINIO_ADDRESS}/machine-controller/${GIT_BRANCH}/${GIT_HEAD_HASH}-${GO_VERSION}.tar"

echo "Upload complete."
