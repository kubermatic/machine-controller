# machine-controller SDK

This directory contains the `k8c.io/machine-controller/sdk` Go module. If you're
looking at integrating the machine controller (MC) into your application, this
is where you should start.

## Usage

Simply `go get` the SDK to use it in your application:

```shell
go get k8c.io/machine-controller/sdk
```

If necessary, you can also import the main MC module, but this comes with heavy
dependencies that might be too costly to maintain for you:

```shell
go get k8c.io/machine-controller
go get k8c.io/machine-controller/sdk
```

In this case it's recommended to always keep both dependencies on the exact same
version.

## Development

There are two main design criteria for the SDK:

1. The SDK should contain a minimal set of dependencies, in a perfect world it
   would be only Kube dependencies. The idea behind the SDK is to make importing
   KKP cheap and easy and to not force dependencies onto consumers.

1. The SDK should not contain as few functions as possible. Functions always
   represent application logic and usually that logic should not be hardcoded into
   client apps. Every function in the SDK is therefore to be considered "eternal".

1. The SDK should truly follow the Go Modules idea of declaring the _minimum_
   compatible versions of every dependency and even of Go. The main machine
   controller module can and should have the _latest_ dependencies, but the SDK
   should not force consumers to be on the most recent Kube version, for example.
