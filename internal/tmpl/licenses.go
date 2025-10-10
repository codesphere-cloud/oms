// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package tmpl

import (
	_ "embed"
)

//go:generate cp ../../LICENSE .
//go:embed LICENSE
var license string

//go:generate cp ../../NOTICE .
//go:embed NOTICE
var notice string

func License() string {
	return license
}

func Notice() string {
	return notice
}
