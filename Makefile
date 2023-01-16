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

SHELL = /bin/bash -eu -o pipefail

GO_VERSION ?= 1.19.4

GOOS ?= $(shell go env GOOS)

export CGO_ENABLED := 0

export GIT_TAG ?= $(shell git tag --points-at HEAD)

export GOFLAGS?=-mod=readonly -trimpath

REGISTRY ?= quay.io
REGISTRY_NAMESPACE ?= kubermatic

LDFLAGS ?= -ldflags '-s -w'

IMAGE_TAG = \
		$(shell echo $$(git rev-parse HEAD && if [[ -n $$(git status --porcelain) ]]; then echo '-dirty'; fi)|tr -d ' ')
IMAGE_NAME ?= $(REGISTRY)/$(REGISTRY_NAMESPACE)/machine-controller:$(IMAGE_TAG)

OS = amzn2 centos ubuntu rhel flatcar rockylinux
USERDATA_BIN = $(patsubst %, machine-controller-userdata-%, $(OS))

BASE64_ENC = \
		$(shell if base64 -w0 <(echo "") &> /dev/null; then echo "base64 -w0"; else echo "base64 -b0"; fi)

.PHONY: all
all: build-machine-controller webhook

.PHONY: build-machine-controller
build-machine-controller: machine-controller $(USERDATA_BIN)

machine-controller-userdata-%: cmd/userdata/% $(shell find cmd/userdata/$* pkg -name '*.go')
	GOOS=$(GOOS) go build -v \
		$(LDFLAGS) \
		-o $@ \
		github.com/kubermatic/machine-controller/cmd/userdata/$*

%: cmd/% $(shell find cmd/$* pkg -name '*.go')
	GOOS=$(GOOS) go build -v \
		$(LDFLAGS) \
		-o $@ \
		github.com/kubermatic/machine-controller/cmd/$*

.PHONY: clean
clean: clean-certs
	rm -f machine-controller \
		webhook \
		$(USERDATA_BIN)

.PHONY: lint
lint:
	golangci-lint run -v

.PHONY: docker-image
docker-image:
	docker build --build-arg GO_VERSION=$(GO_VERSION) -t $(IMAGE_NAME) .

.PHONY: docker-image-publish
docker-image-publish: docker-image
	docker push $(IMAGE_NAME)
	if [[ -n "$(GIT_TAG)" ]]; then \
		$(eval IMAGE_TAG = $(GIT_TAG)) \
		docker build -t $(IMAGE_NAME) . && \
		docker push $(IMAGE_NAME) && \
		$(eval IMAGE_TAG = latest) \
		docker build -t $(IMAGE_NAME) . ;\
		docker push $(IMAGE_NAME) ;\
	fi

.PHONY: test-unit-docker
test-unit-docker:
	@docker run --rm \
		-v $$PWD:/go/src/github.com/kubermatic/machine-controller \
		-v $$PWD/.buildcache:/cache \
		-e GOCACHE=/cache \
		-w /go/src/github.com/kubermatic/machine-controller \
		golang:$(GO_VERSION) \
			make test-unit "GOFLAGS=$(GOFLAGS)"

.PHONY: test-unit
test-unit:
	go test -v ./...

.PHONY: build-tests
build-tests:
	go test -run nope ./...
	go test -tags e2e -run nope ./...

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
		-config examples/webhook-certificate.cnf -extensions v3_req \
		-out examples/admission.csr
	openssl x509 -req \
		-sha256 \
		-days 10000 \
		-extensions v3_req \
		-extfile examples/webhook-certificate.cnf \
		-in examples/admission.csr \
		-CA examples/ca-cert.pem \
		-CAkey examples/ca-key.pem \
		-CAcreateserial \
		-out examples/admission-cert.pem

clean-certs:
	cd examples/ && rm -f admission.csr admission-cert.pem admission-key.pem ca-cert.pem ca-key.pem

.PHONY: deploy
deploy: examples/admission-cert.pem
	@cat examples/machine-controller.yaml \
		|sed "s/__admission_ca_cert__/$(shell cat examples/ca-cert.pem|$(BASE64_ENC))/g" \
		|sed "s/__admission_cert__/$(shell cat examples/admission-cert.pem|$(BASE64_ENC))/g" \
		|sed "s/__admission_key__/$(shell cat examples/admission-key.pem|$(BASE64_ENC))/g" \
		|kubectl apply -f -

.PHONY: check-dependencies
check-dependencies:
	go mod verify

.PHONY: download-gocache
download-gocache:
	@./hack/ci/download-gocache.sh

.PHONY: shfmt
shfmt:
	shfmt -w -sr -i 2 hack
