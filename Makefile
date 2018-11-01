SHELL = /bin/bash

GO_VERSION = 1.10.1

export CGO_ENABLED := 0

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

webhook: $(shell find cmd pkg -name '*.go') vendor
	go build -v \
		-ldflags '-s -w' \
		-o webhook \
		github.com/kubermatic/machine-controller/cmd/webhook

lint:
	gometalinter --config gometalinter.json ./...

docker-image: machine-controller webhook docker-image-nodep

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

e2e-cluster:
	make -C test/tools/integration apply
	./test/tools/integration/provision_master.sh do-not-deploy-machine-controller

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
