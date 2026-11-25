// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point of the OMS CLI. It delegates to the Cobra root command in cli/cmd.
package main

import (
	"github.com/codesphere-cloud/oms/cli/cmd"
)

func main() {
	cmd.Execute()
}
