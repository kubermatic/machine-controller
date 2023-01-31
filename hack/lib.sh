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

### Contains commonly used functions for the other scripts.

# Required for signal propagation to work so
# the cleanup trap gets executed when a script
# receives a SIGINT
set -o monitor

# appendTrap appends to existing traps, if any. It is needed because Bash replaces existing handlers
# rather than appending: https://stackoverflow.com/questions/3338030/multiple-bash-traps-for-the-same-signal
# Needing this func is a strong indicator that Bash is not the right language anymore. Also, this
# basically needs unit tests.
appendTrap() {
  command="$1"
  signal="$2"

  # Have existing traps, must append
  if [[ "$(trap -p | grep $signal)" ]]; then
    existingHandlerName="$(trap -p | grep $signal | awk '{print $3}' | tr -d "'")"

    newHandlerName="${command}_$(head /dev/urandom | tr -dc A-Za-z0-9 | head -c 13)"
    # Need eval to get a random func name
    eval "$newHandlerName() { $command; $existingHandlerName; }"
    echodate "Appending $command as trap for $signal, existing command $existingHandlerName"
    trap $newHandlerName $signal
  # First trap
  else
    echodate "Using $command as trap for $signal"
    trap $command $signal
  fi
}

containerize() {
  local cmd="$1"
  local image="${CONTAINERIZE_IMAGE:-quay.io/kubermatic/util:2.0.0}"
  local gocache="${CONTAINERIZE_GOCACHE:-/tmp/.gocache}"
  local gomodcache="${CONTAINERIZE_GOMODCACHE:-/tmp/.gomodcache}"
  local skip="${NO_CONTAINERIZE:-}"

  # short-circuit containerize when in some cases it needs to be avoided
  [ -n "$skip" ] && return

  if ! [ -f /.dockerenv ]; then
    echodate "Running $cmd in a Docker container using $image..."
    mkdir -p "$gocache"
    mkdir -p "$gomodcache"

    exec docker run \
      -v "$PWD":/go/src/k8c.io/kubermatic \
      -v "$gocache":"$gocache" \
      -v "$gomodcache":"$gomodcache" \
      -w /go/src/k8c.io/kubermatic \
      -e "GOCACHE=$gocache" \
      -e "GOMODCACHE=$gomodcache" \
      -u "$(id -u):$(id -g)" \
      --entrypoint="$cmd" \
      --rm \
      -it \
      $image $@

    exit $?
  fi
}

echodate() {
  # do not use -Is to keep this compatible with macOS
  echo "[$(date +%Y-%m-%dT%H:%M:%S%:z)]" "$@"
}

# returns the current time as a number of milliseconds
nowms() {
  echo $(($(date +%s%N) / 1000000))
}

# returns the number of milliseconds elapsed since the given time
elapsed() {
  echo $(($(nowms) - $1))
}

# pushes a Prometheus metric to a pushgateway
pushMetric() {
  local metric="$1"
  local value="$2"
  local labels="${3:-}"
  local kind="${4:-gauge}"
  local help="${5:-}"
  local pushgateway="${PUSHGATEWAY_URL:-}"
  local job="ci"
  local instance="${PROW_JOB_ID:-}"
  local prowjob="${JOB_NAME:-}"

  if [ -z "$pushgateway" ]; then
    return
  fi

  local payload="# TYPE $metric $kind"

  if [ -n "$help" ]; then
    payload="$payload\n# HELP $metric $help"
  fi

  if [ -n "$labels" ]; then
    labels=",$labels"
  fi

  payload="$payload\n$metric{prowjob=\"$prowjob\"$labels} $value\n"

  echo -e "$payload" | curl --data-binary @- -s "$pushgateway/metrics/job/$job/instance/$instance"
}

pushElapsed() {
  pushMetric "$1" $(elapsed $2) "${3:-}" "${4:-}" "${5:-}"
}

retry() {
  # Works only with bash but doesn't fail on other shells
  start_time=$(date +%s)
  set +e
  actual_retry $@
  rc=$?
  set -e
  elapsed_time=$(($(date +%s) - $start_time))
  write_junit "$rc" "$elapsed_time"
  return $rc
}

write_junit() {
  # Doesn't make any sense if we don't know a testname
  if [ -z "${TEST_NAME:-}" ]; then return; fi
  # Only run in CI
  if [ -z "${ARTIFACTS:-}" ]; then return; fi

  rc=$1
  duration=${2:-0}
  errors=0
  failure=""
  if [ "$rc" -ne 0 ]; then
    errors=1
    failure='<failure type="Failure">Step failed</failure>'
  fi
  TEST_CLASS="${TEST_CLASS:-Kubermatic}"
  cat << EOF > ${ARTIFACTS}/junit.$(echo $TEST_NAME | sed 's/ /_/g' | tr '[:upper:]' '[:lower:]').xml
<?xml version="1.0" ?>
<testsuites>
  <testsuite errors="$errors" failures="$errors" name="$TEST_CLASS" tests="1">
    <testcase classname="$TEST_CLASS" name="$TEST_NAME" time="$duration">
      $failure
    </testcase>
  </testsuite>
</testsuites>
EOF
}

# We use an extra wrapping to write junit and have a timer
actual_retry() {
  retries=$1
  shift

  count=0
  delay=1
  until "$@"; do
    rc=$?
    count=$((count + 1))
    if [ $count -lt "$retries" ]; then
      echo "Retry $count/$retries exited $rc, retrying in $delay seconds..." > /dev/stderr
      sleep $delay
    else
      echo "Retry $count/$retries exited $rc, no more retries left." > /dev/stderr
      return $rc
    fi
    delay=$((delay * 2))
  done
  return 0
}

start_docker_daemon_ci() {
  # DOCKER_REGISTRY_MIRROR_ADDR is injected via Prow preset;
  # start-docker.sh is part of the build image.
  DOCKER_REGISTRY_MIRROR="${DOCKER_REGISTRY_MIRROR_ADDR:-}" DOCKER_MTU=1400 start-docker.sh
}

start_docker_daemon() {
  if docker stats --no-stream > /dev/null 2>&1; then
    echodate "Not starting Docker again, it's already running."
    return
  fi

  # Start Docker daemon
  echodate "Starting Docker"
  dockerd > /tmp/docker.log 2>&1 &

  echodate "Started Docker successfully"
  appendTrap docker_logs EXIT

  # Wait for Docker to start
  echodate "Waiting for Docker"
  retry 5 docker stats --no-stream
  echodate "Docker became ready"
}

check_all_deployments_ready() {
  local namespace="$1"

  # check that Deployments have been created
  local deployments
  deployments=$(kubectl -n $namespace get deployments -o json)

  if [ $(echo "$deployments" | jq '.items | length') -eq 0 ]; then
    echodate "No Deployments created yet."
    return 1
  fi

  # check that all Deployments are ready
  local unready
  unready=$(echo "$deployments" | jq -r '[.items[] | select(.spec.replicas > 0) | select (.status.availableReplicas < .spec.replicas) | .metadata.name] | @tsv')
  if [ -n "$unready" ]; then
    echodate "Not all Deployments have finished rolling out, namely: $unready"
    return 1
  fi

  return 0
}
