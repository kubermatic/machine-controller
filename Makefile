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

GO_VERSION ?= 1.27.4

GOOS ?= $(shell go env GOOS)

export CGO_ENABLED := 0

export GIT_TAG ?= $(shell git tag --points-at HEAD 'v*')

export GOFLAGS?=-mod=readonly -trimpath

REGISTRY ?= quay.io
REGISTRY_NAMESPACE ?= kubermatic

LDFLAGS ?= -ldflags '-s -w'

IMAGE_TAG = \
		$(shell echo $$(git rev-parse HEAD && if [[ -n $$(git status --porcelain) ]]; then echo '-dirty'; fi)|tr -d ' ')
IMAGE_REPO ?= $(REGISTRY)/$(REGISTRY_NAMESPACE)/machine-controller
IMAGE_NAME ?= $(IMAGE_REPO):$(IMAGE_TAG)

# Platforms for the published multi-arch image.
PLATFORMS ?= linux/amd64,linux/arm64

# GOPROXY without a value is taken from the environment, if set.
DOCKER_BUILD_ARGS = --build-arg GO_VERSION=$(GO_VERSION) --build-arg GOPROXY

OS = amzn2 ubuntu rhel flatcar rockylinux

BASE64_ENC = \
		$(shell if base64 -w0 <(echo "") &> /dev/null; then echo "base64 -w0"; else echo "base64 -b0"; fi)

.PHONY: all
all: build-machine-controller webhook

.PHONY: build-machine-controller
build-machine-controller: machine-controller

%: cmd/% $(shell find cmd/$* pkg -name '*.go')
	GOOS=$(GOOS) go build -v \
		$(LDFLAGS) \
		-o $@ \
		k8c.io/machine-controller/cmd/$*

.PHONY: clean
clean:
	rm -f machine-controller webhook

.PHONY: lint
lint:
	golangci-lint run -v
	make -C sdk lint

yamllint:
	yamllint -c .yamllint.conf .

# Single-arch image for the host platform, loaded into the local image store.
.PHONY: docker-image
docker-image:
	docker build $(DOCKER_BUILD_ARGS) -t $(IMAGE_NAME) .

# Builds the image for all $(PLATFORMS) without pushing or loading it.
# Requires a buildx builder with multi-platform support (see hack/ci/setup-docker-buildx.sh).
.PHONY: docker-image-multiarch
docker-image-multiarch:
	docker buildx build $(DOCKER_BUILD_ARGS) --platform $(PLATFORMS) .

# Builds and pushes the multi-arch image in one go, tagged with the commit SHA and,
# for tagged commits, additionally with the git tag(s) and "latest". All tags point to
# the same index, so the per-platform SBOMs only need to be attached once.
# Requires a buildx builder with multi-platform support (see hack/ci/setup-docker-buildx.sh).
.PHONY: docker-image-publish
docker-image-publish:
	docker buildx build $(DOCKER_BUILD_ARGS) --platform $(PLATFORMS) \
		--tag $(IMAGE_NAME) \
		$(foreach tag,$(GIT_TAG) $(if $(GIT_TAG),latest),--tag $(IMAGE_REPO):$(tag)) \
		--push .
	./hack/attach-image-sbom.sh $(IMAGE_NAME)

.PHONY: test-unit-docker
test-unit-docker:
	@docker run --rm \
		-v $$PWD:/go/src/k8c.io/machine-controller \
		-v $$PWD/.buildcache:/cache \
		-e GOCACHE=/cache \
		-w /go/src/k8c.io/machine-controller \
		golang:$(GO_VERSION) \
			make test-unit "GOFLAGS=$(GOFLAGS)"

.PHONY: test-unit
test-unit:
	go test -v ./...
	cd sdk && go test -v ./...

.PHONY: build-tests
build-tests:
	go test -run nope ./...
	cd sdk && go test -run nope ./...
	go test -tags e2e -run nope ./...

.PHONY: check-dependencies
check-dependencies:
	go mod verify
	cd sdk && go mod verify

.PHONY: download-gocache
download-gocache:
	@./hack/ci/download-gocache.sh

.PHONY: shfmt
shfmt:
	shfmt -w -sr -i 2 hack
