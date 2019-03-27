SHELL = /bin/bash

GO_VERSION = 1.10.3

export CGO_ENABLED := 0

export E2E_SSH_PUBKEY ?= $(shell cat ~/.ssh/id_rsa.pub)

export GIT_TAG ?= $(shell git tag --points-at HEAD)

REGISTRY ?= docker.io
REGISTRY_NAMESPACE ?= kubermatic

IMAGE_TAG = \
		$(shell echo $$(git rev-parse HEAD && if [[ -n $$(git status --porcelain) ]]; then echo '-dirty'; fi)|tr -d ' ')
IMAGE_NAME = $(REGISTRY)/$(REGISTRY_NAMESPACE)/machine-controller:$(IMAGE_TAG)


vendor: Gopkg.lock Gopkg.toml
	dep ensure -vendor-only

machine-controller-docker:
	@docker run --rm \
		-v $$PWD:/go/src/github.com/kubermatic/machine-controller \
		-v $$PWD/.buildcache:/cache \
		-e GOCACHE=/cache \
		-w /go/src/github.com/kubermatic/machine-controller \
		golang:$(GO_VERSION) \
			make machine-controller

machine-controller: $(shell find cmd pkg -name '*.go') vendor
	go build -v \
		-ldflags '-s -w' \
		-o machine-controller \
		github.com/kubermatic/machine-controller/cmd/controller
	go build -v \
		-ldflags '-s -w' \
		-o machine-controller-userdata-centos \
		github.com/kubermatic/machine-controller/cmd/userdata/centos
	go build -v \
		-ldflags '-s -w' \
		-o machine-controller-userdata-coreos \
		github.com/kubermatic/machine-controller/cmd/userdata/coreos
	go build -v \
		-ldflags '-s -w' \
		-o machine-controller-userdata-ubuntu \
		github.com/kubermatic/machine-controller/cmd/userdata/ubuntu

webhook: $(shell find cmd pkg -name '*.go') vendor
	go build -v \
		-ldflags '-s -w' \
		-o webhook \
		github.com/kubermatic/machine-controller/cmd/webhook

lint:
	./hack/verify-type-revision-annotation-const.sh
	golangci-lint run

docker-image: machine-controller webhook
	docker build -t $(IMAGE_NAME) .
	docker push $(IMAGE_NAME)
	if [[ -n "$(GIT_TAG)" ]]; then \
		$(eval IMAGE_TAG = $(GIT_TAG)) \
		docker build -t $(IMAGE_NAME) . && \
		docker push $(IMAGE_NAME) && \
		$(eval IMAGE_TAG = latest) \
		docker build -t $(IMAGE_NAME) . ;\
		docker push $(IMAGE_NAME) ;\
	fi

test-unit-docker:
	@docker run --rm \
		-v $$PWD:/go/src/github.com/kubermatic/machine-controller \
		-v $$PWD/.buildcache:/cache \
		-e GOCACHE=/cache \
		-w /go/src/github.com/kubermatic/machine-controller \
		golang:$(GO_VERSION) \
			make test-unit

test-unit: vendor
	@#The `-race` flag requires CGO
	CGO_ENABLED=1 go test -race ./...

e2e-cluster: machine-controller webhook
	make -C test/tools/integration apply
	./test/tools/integration/provision_master.sh do-not-deploy-machine-controller
	KUBECONFIG=$(shell pwd)/.kubeconfig kubectl apply -f examples/machine-controller.yaml -l local-testing="true"

e2e-destroy:
	./test/tools/integration/cleanup_machines.sh
	make -C test/tools/integration destroy

examples/ca-key.pem:
	openssl genrsa -out examples/ca-key.pem 4096

examples/ca-cert.pem: examples/ca-key.pem
	openssl req -x509 -new -nodes -key examples/ca-key.pem \
    -subj "/C=US/ST=CA/O=Acme/CN=k8s-machine-controller-ca" \
		-sha256 -days 10000 -out examples/ca-cert.pem

examples/admission-key.pem: examples/ca-cert.pem
	openssl genrsa -out examples/admission-key.pem 2048
	chmod 0600 examples/admission-key.pem

examples/admission-cert.pem: examples/admission-key.pem
	openssl req -new -sha256 \
    -key examples/admission-key.pem \
    -subj "/C=US/ST=CA/O=Acme/CN=machine-controller-webhook.kube-system.svc" \
    -out examples/admission.csr
	openssl x509 -req -in examples/admission.csr -CA examples/ca-cert.pem \
		-CAkey examples/ca-key.pem -CAcreateserial \
		-out examples/admission-cert.pem -days 10000 -sha256

deploy: examples/admission-cert.pem
	@cat examples/machine-controller.yaml \
		|sed "s/__admission_ca_cert__/$(shell cat examples/ca-cert.pem|base64 -w0)/g" \
		|sed "s/__admission_cert__/$(shell cat examples/admission-cert.pem|base64 -w0)/g" \
		|sed "s/__admission_key__/$(shell cat examples/admission-key.pem|base64 -w0)/g" \
		|kubectl apply -f -

check-dependencies:
	# We need mercurial for bitbucket.org/ww/goautoneg, otherwise dep hangs forever
	which hg >/dev/null 2>&1 || apt update && apt install -y mercurial
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep status
