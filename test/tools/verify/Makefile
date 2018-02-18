GO_VERSION = 1.10.0

verify: cmd/*
		@docker run --rm \
			-v $$PWD/../../..:/go/src/github.com/kubermatic/machine-controller \
			-v $$PWD/../../../.buildcache:/cache \
			-e GOCACHE=/cache \
			-w /go/src/github.com/kubermatic/machine-controller/test/tools/verify \
			golang:$(GO_VERSION) \
			env CGO_ENABLED=0 go build \
				-ldflags '-s -w' \
				-o verify \
				github.com/kubermatic/machine-controller/test/tools/verify/cmd
