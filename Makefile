machine-controller: cmd pkg vendor
		@docker run --rm \
			-v $$PWD:/go/src/github.com/kubermatic/machine-controller \
			-w /go/src/github.com/kubermatic/machine-controller \
			golang:1.9.2 \
			env CGO_ENABLED=0 go build -o machine-controller cmd/controller/main.go

docker-image:
	docker build -t machine-controller .
