verify: cmd/*
		@docker run --rm \
			-v $$PWD/../../..:/go/src/github.com/kubermatic/machine-controller \
			-w /go/src/github.com/kubermatic/machine-controller/test/tools/verify \
			golang:1.9.2 \
			env CGO_ENABLED=0 go build \
				-ldflags '-s -w' \
				-o verify \
				github.com/kubermatic/machine-controller/test/tools/verify/cmd
