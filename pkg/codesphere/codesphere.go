/*
Copyright © 2025 Codesphere Inc.
*/

package codesphere

import "fmt"

type Codesphere struct {
	Name *string
}

func (cs Codesphere) String() string {
	return fmt.Sprintf("Codesphere details:\n   Name: %s", *cs.Name)
}
