GO_VERSION = 1.10.0

USE_VOLUME_CONTAINER ?= false

# Required because circlecCI can not use
# Docker volumes, so we have to use a volume
# container instead there
ifeq ($(USE_VOLUME_CONTAINER),true)
	VOL_ARG = --volumes-from vol-container
else
	VOL_ARG = -v $$PWD/../../..:/go/src/github.com/kubermatic/machine-controller
endif

verify: cmd/*
		@docker run --rm \
			$(VOL_ARG) \
			-v $$PWD/../../../.buildcache:/cache \
			-e GOCACHE=/cache \
			-w /go/src/github.com/kubermatic/machine-controller/test/tools/verify \
			golang:$(GO_VERSION) \
			env CGO_ENABLED=0 go build \
				-ldflags '-s -w' \
				-o verify \
				github.com/kubermatic/machine-controller/test/tools/verify/cmd
