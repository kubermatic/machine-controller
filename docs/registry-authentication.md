# Registry Authentication

Machine-controller supports configuring container-runtime with authentication
information. Flag `-node-registry-credentials-secret` can take a secret
reference where authentication info will be stored. During VM creation this info
will be used to configure container-runtime.

Secret format is serialized `map[string]github.com/containerd/containerd/pkg/cri/config.AuthConfig`

example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-registries
  namespace: kube-system
data:
  gcr.io: |
    eyJ1c2VybmFtZSI6ImwwZzFuIiwicGFzc3dvcmQiOiJjMDBscDQ1NXc
    wcmQiLCJhdXRoIjoiIiwiaWRlbnRpdHl0b2tlbiI6IiJ9Cg==

```

Now having this saved in kubernetes API, launch machine-controller with
`-node-registry-credentials-secret=kube-system/my-registries` flag.
