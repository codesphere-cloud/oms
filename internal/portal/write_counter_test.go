// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"bytes"
	"log"
	"testing"
	"time"
)

func TestWriteCounterEmitsProgress(t *testing.T) {
	// capture log output
	var logBuf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prev)

	var underlying bytes.Buffer
	wc := NewWriteCounter(&underlying)

	// force an update by setting LastUpdate sufficiently in the past
	wc.LastUpdate = time.Now().Add(-200 * time.Millisecond)

	_, err := wc.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	out := logBuf.String()
	if out == "" {
		t.Fatalf("expected progress log output, got none")
	}
}
