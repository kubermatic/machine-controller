SHELL = /bin/bash

REGISTRY ?= docker.io
REGISTRY_NAMESPACE ?= kubermatic

IMAGE_TAG = \
		$(shell echo $$(git rev-parse HEAD && if [[ -n $$(git status --porcelain) ]]; then echo '-dirty'; fi)|tr -d ' ')
IMAGE_NAME = $(REGISTRY)/$(REGISTRY_NAMESPACE)/machine-controller:$(IMAGE_TAG)

vendor: Gopkg.lock Gopkg.toml
	dep ensure -vendor-only

machine-controller: $(shell find cmd pkg -name '*.go') vendor
		@docker run --rm \
			-v $$PWD:/go/src/github.com/kubermatic/machine-controller \
			-w /go/src/github.com/kubermatic/machine-controller \
			golang:1.9.2 \
			env CGO_ENABLED=0 go build \
				-ldflags '-s -w' \
				-o machine-controller \
				github.com/kubermatic/machine-controller/cmd/controller

docker-image: machine-controller
	make docker-image-nodep

# This target exists because in our CI
# we do not want to restore the vendor
# folder for the push step, but we know
# for sure it is not required there
docker-image-nodep:
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

test-e2e:
	cd test && make hetzner-e2e
