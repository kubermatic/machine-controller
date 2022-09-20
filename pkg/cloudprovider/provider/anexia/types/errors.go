/*
Copyright 2022 The Machine Controller Authors.

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
