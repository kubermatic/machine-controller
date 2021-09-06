package types

import (
	"fmt"
	"strings"
)

// MultiError represent multiple errors at the same time.
type MultiError []error

func (r MultiError) Error() string {
	errString := make([]string, len(r))
	for i, err := range r {
		errString[i] = fmt.Sprintf("Error %d: %s", i, err)
	}
	return fmt.Sprintf("Multiple errors occoured:\n%s", strings.Join(errString, "\n"))
}

func NewMultiError(errs ...error) error {
	var combinedErr []error
	for _, err := range errs {
		if err == nil {
			continue
		}
		combinedErr = append(combinedErr, err)
	}

	if len(combinedErr) > 0 {
		return MultiError(combinedErr)
	}

	return nil
}
