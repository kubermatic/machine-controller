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

package helper

import (
	"flag"

	"github.com/Masterminds/semver"
)

var update = flag.Bool("update", false, "update testdata files")

var (
	versions = []*semver.Version{
		semver.MustParse("v1.10.0"),
		semver.MustParse("v1.11.0"),
		semver.MustParse("v1.11.0-rc.2"),
		semver.MustParse("v1.11.3"),
		semver.MustParse("v1.12.0"),
	}
)
