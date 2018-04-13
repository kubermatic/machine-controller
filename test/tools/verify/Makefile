GO_VERSION = 1.10.0

export GOCACHE := $(PWD)/../../../.buildcache
export CGO_ENABLED := 0

verify-docker:
	@docker run --rm \
		-v $(PWD)/../../..:/go/src/github.com/kubermatic/machine-controller \
		-w /go/src/github.com/kubermatic/machine-controller/test/tools/verify \
		golang:$(GO_VERSION) \
		make verify

verify: cmd/*
	go build \
		-ldflags '-s -w' \
		-o verify \
		github.com/kubermatic/machine-controller/test/tools/verify/cmd
