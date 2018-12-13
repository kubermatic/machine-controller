package helper

import (
	"flag"

	"github.com/Masterminds/semver"
)

var update = flag.Bool("update", false, "update .golden files")

var (
	Versions = []*semver.Version{
		semver.MustParse("v1.10.0"),
		semver.MustParse("v1.11.0"),
		semver.MustParse("v1.11.0-rc.2"),
		semver.MustParse("v1.11.3"),
		semver.MustParse("v1.12.0"),
		semver.MustParse("v1.13.0"),
	}
)
