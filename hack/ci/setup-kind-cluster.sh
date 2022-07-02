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

source hack/lib.sh

echodate "Setting up kind cluster..."

if [ -z "${JOB_NAME:-}" ] || [ -z "${PROW_JOB_ID:-}" ]; then
  echodate "This script should only be running in a CI environment."
  exit 1
fi

export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-machine-controller}"

start_docker_daemon_ci

# Make debugging a bit better
echodate "Configuring bash"
cat << EOF >> ~/.bashrc
# Gets set to the CI clusters kubeconfig from a preset
unset KUBECONFIG

cn() {
  kubectl config set-context --current --namespace=\$1
}

kubeconfig() {
  TMP_KUBECONFIG=\$(mktemp);
  kubectl get secret admin-kubeconfig -o go-template='{{ index .data "kubeconfig" }}' | base64 -d > \$TMP_KUBECONFIG;
  export KUBECONFIG=\$TMP_KUBECONFIG;
  cn kube-system
}

# this alias makes it so that watch can be used with other aliases, like "watch k get pods"
alias watch='watch '
alias k=kubectl
alias ll='ls -lh --file-type --group-directories-first'
alias lll='ls -lahF --group-directories-first'
source <(k completion bash )
source <(k completion bash | sed s/kubectl/k/g)
EOF

# Find external IP of node where this pod is running
echodate "Retrieving the external node IP where this pod is scheduled"
export NODE_NAME=$(kubectl --kubeconfig /etc/kubeconfig/kubeconfig get pods -l prow.k8s.io/id=$PROW_JOB_ID -o jsonpath="{.items..spec.nodeName}")
export NODE_IP=$(kubectl --kubeconfig /etc/kubeconfig/kubeconfig get node $NODE_NAME -o jsonpath="{.status.addresses[?(@.type=='ExternalIP')].address}")

if [ -z "$NODE_NAME" ] || [ -z "$NODE_IP" ]; then
  echodate "This script was unable to determine the external IP for kube-apiserver."
  exit 1
fi

# Create kind cluster
TEST_NAME="Create kind cluster"

echodate "Preloading the kindest/node image"
docker load --input /kindest.tar

echodate "Creating the kind cluster"
export KUBECONFIG=~/.kube/config

beforeKindCreate=$(nowms)

cat << EOF > kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: "${KIND_CLUSTER_NAME}"
networking:
  apiServerAddress: "0.0.0.0"
  disableDefaultCNI: true # disable kindnet
kubeadmConfigPatches:
- |
  kind: ClusterConfiguration
  apiServer:
    extraArgs:
      "kubernetes-service-node-port": "31443"
    certSANs:
      - localhost
      - 127.0.0.1
      - kubernetes
      - kubernetes.default.svc
      - kubernetes.default.svc.cluster.local
      - 0.0.0.0
      - ${NODE_IP}
      - ${KIND_CLUSTER_NAME}
nodes:
  - role: control-plane
EOF

if [ -n "${DOCKER_REGISTRY_MIRROR_ADDR:-}" ]; then
  mirrorHost="$(echo "$DOCKER_REGISTRY_MIRROR_ADDR" | sed 's#http://##' | sed 's#/+$##g')"

  # make the registry mirror available as a socket,
  # so we can mount it into the kind cluster
  mkdir -p /mirror
  socat UNIX-LISTEN:/mirror/mirror.sock,fork,reuseaddr,unlink-early,mode=777 TCP4:$mirrorHost &

  function end_socat_process {
    echodate "Killing socat docker registry mirror processes..."
    pkill -e socat
  }
  appendTrap end_socat_process EXIT

  cat << EOF >> kind-config.yaml
    # mount the socket
    extraMounts:
    - hostPath: /mirror
      containerPath: /mirror
containerdConfigPatches:
  # point to the soon-to-start local socat process
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
    endpoint = ["http://127.0.0.1:5001"]
EOF

  kind create cluster --config kind-config.yaml
  pushElapsed kind_cluster_create_duration_milliseconds $beforeKindCreate

  # unwrap the socket inside the kind cluster and make it available on a TCP port,
  # because containerd/Docker doesn't support sockets for mirrors.
  docker exec $KIND_CLUSTER_NAME-control-plane bash -c 'socat TCP4-LISTEN:5001,fork,reuseaddr UNIX:/mirror/mirror.sock &'
else
  kind create cluster --config kind-config.yaml
fi

echodate "Kind cluster $KIND_CLUSTER_NAME is up and running."

if [ ! -f cni-plugin-deployed ]; then
  echodate "Installing CNI plugin."
  (
    # Install CNI plugins since they are not installed by default in KIND. Also, kube-flannel doesn't install
    # CNI plugins unlike other plugins so we have to do it manually.
    setup_cni_in_kind=$(cat hack/ci/setup-cni-in-kind.sh)
    docker exec $KIND_CLUSTER_NAME-control-plane bash -c "$setup_cni_in_kind &"
  )
  kubectl create -f https://raw.githubusercontent.com/flannel-io/flannel/v0.18.0/Documentation/kube-flannel.yml
  touch cni-plugin-deployed
fi

if [ -z "${DISABLE_CLUSTER_EXPOSER:-}" ]; then
  # Annotate kube-apiserver service so that the cluster exposer can expose it
  kubectl annotate svc kubernetes -n default nodeport-proxy.k8s.io/expose=true

  # Start cluster exposer, which will expose services from within kind as
  # a NodePort service on the host
  echodate "Starting cluster exposer"
  (
    # Clone kubermatic repo to build clusterexposer
    mkdir -p /tmp/kubermatic
    cd /tmp/kubermatic
    echodate "Cloning cluster exposer"
    KKP_REPO_URL="${KKP_REPO_URL:-https://github.com/kubermatic/kubermatic.git}"
    KKP_REPO_TAG="${KKP_REPO_BRANCH:-master}"
    git clone --depth 1 --branch "${KKP_REPO_TAG}" "${KKP_REPO_URL}" .

    echodate "Building cluster exposer"
    CGO_ENABLED=0 go build --tags ce -v -o /tmp/clusterexposer ./pkg/test/clusterexposer/cmd
  )

  export KUBECONFIG=~/.kube/config
  /tmp/clusterexposer \
    --kubeconfig-inner "$KUBECONFIG" \
    --kubeconfig-outer "/etc/kubeconfig/kubeconfig" \
    --build-id "$PROW_JOB_ID" &> /var/log/clusterexposer.log &

  function print_cluster_exposer_logs {
    if [[ $? -ne 0 ]]; then
      # Tolerate errors and just continue
      set +e
      echodate "Printing cluster exposer logs"
      cat /var/log/clusterexposer.log
      echodate "Done printing cluster exposer logs"
      set -e
    fi
  }
  appendTrap print_cluster_exposer_logs EXIT

  TEST_NAME="Wait for cluster exposer"
  echodate "Waiting for cluster exposer to be running"

  retry 10 curl -s --fail http://127.0.0.1:2047/metrics -o /dev/null
  echodate "Cluster exposer is running"

  echodate "Setting up iptables rules to make nodeports available"
  KIND_NETWORK_IF=$(ip -br addr | grep -- 'br-' | cut -d' ' -f1)
  KIND_CONTAINER_IP=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $KIND_CLUSTER_NAME-control-plane)

  iptables -t nat -A PREROUTING -i eth0 -p tcp -m multiport --dports=30000:33000 -j DNAT --to-destination $KIND_CONTAINER_IP
  # By default all traffic gets dropped unless specified (tested with docker server 18.09.1)
  iptables -t filter -I DOCKER-USER -d $KIND_CONTAINER_IP/32 ! -i $KIND_NETWORK_IF -o $KIND_NETWORK_IF -p tcp -m multiport --dports=30000:33000 -j ACCEPT
  # Docker sets up a MASQUERADE rule for postrouting, so nothing to do for us

  echodate "Successfully set up iptables rules for nodeports"

  # Compute external kube-apiserver address
  # If svc is not found then we need to check cluster-exposer logs
  PORT=$(kubectl --kubeconfig /etc/kubeconfig/kubeconfig get svc -l prow.k8s.io/id=$PROW_JOB_ID -o jsonpath="{.items..spec.ports[0].nodePort}")

  if [ -z "$PORT" ] || [ -z "$NODE_NAME" ] || [ -z "$NODE_IP" ]; then
    echodate "This script was unable to determine the external IP for kube-apiserver."
    exit 1
  fi

  export MASTER_URL="https://$NODE_IP:$PORT"

  retry 5 curl -ks --fail $MASTER_URL/version -o /dev/null
  echodate "New api-server address is reachable"

  # Use kubeconfig with external kube-apiserver address for machine-controller
  cp ~/.kube/config ~/.kube/config-external
  sed -i "s;server.*;server: $MASTER_URL;g" ~/.kube/config-external

  export KUBECONFIG=~/.kube/config-external
  echodate "kube-apiserver for KIND cluster successfully exposed."
fi
