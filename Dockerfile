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

ARG GO_VERSION=1.27.4
# The builder always runs on the build host's platform and cross-compiles
# for the target platform, so multi-arch builds don't need to emulate the Go
# toolchain.
FROM --platform=$BUILDPLATFORM docker.io/golang:${GO_VERSION} AS builder
ARG GOPROXY
ENV GOPROXY=${GOPROXY:-https://proxy.golang.org,direct}
WORKDIR /go/src/k8c.io/machine-controller

# Module download does not depend on the target platform, so this layer is
# shared between all platforms of a multi-arch build.
COPY go.mod go.sum ./
COPY sdk/go.mod sdk/go.sum ./sdk/
RUN go mod download

COPY . .
# Declared after the shared layers; anything below runs once per platform.
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH make all

FROM alpine:3.23

RUN apk upgrade --no-cache && apk add --no-cache ca-certificates cdrkit

COPY --from=builder \
    /go/src/k8c.io/machine-controller/machine-controller \
    /go/src/k8c.io/machine-controller/webhook \
    /usr/local/bin/
USER nobody
