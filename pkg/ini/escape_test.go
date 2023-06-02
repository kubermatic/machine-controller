/*
Copyright 2019 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ini

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/sethvargo/go-password/password"
	"gopkg.in/gcfg.v1"
)

const (
	testTpl = `[Global]
Password = {{ .Global.Password | iniEscape }}
`
)

type globalSection struct {
	Password string
}

type testData struct {
	Global globalSection
}

// TestINIEscape will ensure that we hopefully cover every case.
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

		before := &testData{
			Global: globalSection{
				Password: pw,
			},
		}

		funcMap := sprig.TxtFuncMap()
		funcMap["iniEscape"] = Escape

		tpl, err := template.New("test").Funcs(funcMap).Parse(testTpl)
		if err != nil {
			t.Fatalf("failed to parse the cloud config template: %v", err)
		}

		buf := &bytes.Buffer{}
		if err := tpl.Execute(buf, before); err != nil {
			t.Fatalf("failed to execute cloud config template: %v", err)
		}

		after := &testData{}
		if err := gcfg.ReadStringInto(after, buf.String()); err != nil {
			t.Logf("\n%s", after)
			t.Fatalf("failed to load string into config object: %v", err)
		}

		if before.Global.Password != after.Global.Password {
			t.Fatalf("after unmarshalling the config into a string an reading it back in, the value changed. Password before:\n%s Password after:\n%s", before.Global.Password, after.Global.Password)
		}
	}
}
