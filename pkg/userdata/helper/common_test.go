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

	"github.com/Masterminds/semver/v3"
)

var update = flag.Bool("update", false, "update testdata files")

var (
	versions = []*semver.Version{
		semver.MustParse("v1.26.12"),
		semver.MustParse("v1.27.9"),
		semver.MustParse("v1.28.5"),
		semver.MustParse("v1.29.0"),
	}
)
