# Registry Authentication

Machine-controller supports configuring container runtime with authentication
information. Flag `-node-registry-credentials-secret` can take a secret
reference in form `namespace/secret-name` where authentication info will be
stored. During the VM creation this info will be used to configure container
runtime.

Secret format is serialized
`map[string]github.com/containerd/containerd/pkg/cri/config.AuthConfig`, where
`AuthConfig` is defined as

```go
type AuthConfig struct {
	// Username is the username to login the registry.
	Username string `toml:"username" json:"username"`
	// Password is the password to login the registry.
	Password string `toml:"password" json:"password"`
	// Auth is a base64 encoded string from the concatenation of the username,
	// a colon, and the password.
	Auth string `toml:"auth" json:"auth"`
	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken string `toml:"identitytoken" json:"identitytoken"`
}
```

Original source: https://github.com/containerd/containerd/blob/v1.5.9/pkg/cri/config/config.go#L126-L137


Example:

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

Now having this saved in the Kubernetes API, launch machine-controller with
`-node-registry-credentials-secret=kube-system/my-registries` flag.
