# A Simple verification tool

# Purpose
verifies whether a machine (node) has been created by the machine-controller.

# How does it work
it accepts a machine's manifest and a list of key value pairs, then replaces
keywords under the given key to the given value.

next, the modified manifest is POST'ed to the kube-api server, and the test tool watches
the API and verifies whether the expected machine resource has been created.


# Building
```bash
go build cmd\main.go
```

# Running
```bash
./verify -input path_to_a_manifest -parameters "key=value,key2=value2"
```

for example the following command will replace <<DO_TOKEN>> and <<PUBLIC_KEY>> in the digitalocean manifest:
```bash
./verify -input ../manifests/machine-digitalocean.yaml -parameters "<<DO_TOKEN>>=TEST,<<PUBLIC_KEY>>=TEST2"
```
