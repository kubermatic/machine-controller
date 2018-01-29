package errors

import "errors"

var (
	NoSupportedVersionAvailableErr = errors.New("no supported version available")
	VersionNotAvailableErr         = errors.New("version not available")
)
