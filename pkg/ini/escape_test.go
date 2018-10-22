package ini

import (
	"testing"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/openstack"

	"github.com/sethvargo/go-password/password"
	"gopkg.in/gcfg.v1"
)

// TestINIEscape will ensure that we hopefully cover every case
func TestINIEscape(t *testing.T) {
	// We'll simply generate 1000 times a password with special chars,
	// Put it into a OpenStack cloud config,
	// Marshal it,
	// Unmarshal it,
	// Compare if the input & output password match
	for i := 0; i <= 1000; i++ {
		pw, err := password.Generate(64, 10, len(password.Symbols), false, false)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("testing with pw: %s", pw)

		before := &openstack.CloudConfig{
			Global: openstack.GlobalOpts{
				Password: pw,
			},
		}

		s, err := openstack.CloudConfigToString(before)
		if err != nil {
			t.Fatal(err)
		}

		after := &openstack.CloudConfig{}
		if err := gcfg.ReadStringInto(after, s); err != nil {
			t.Logf("\n%s", s)
			t.Fatalf("failed to load string into config object: %v", err)
		}

		if before.Global.Password != after.Global.Password {
			t.Fatalf("after unmarshalling the config into a string an reading it back in, the value changed")
		}
	}
}
