// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal

import (
	"fmt"
	"io"
	"log"
	"time"
)

// WriteCounter is a custom io.Writer that counts bytes written and logs progress.
type WriteCounter struct {
	Written     int64
	LastUpdate  time.Time
	Writer      io.Writer
	currentAnim int
}

// NewWriteCounter creates a new WriteCounter.
func NewWriteCounter(writer io.Writer) *WriteCounter {
	return &WriteCounter{
		Writer: writer,
		// Initialize to zero so the first Write triggers an immediate log
		LastUpdate: time.Time{},
	}
}

// Write implements the io.Writer interface for WriteCounter.
func (wc *WriteCounter) Write(p []byte) (int, error) {
	// Write the bytes to the underlying writer
	n, err := wc.Writer.Write(p)
	if err != nil {
		return n, err
	}

	wc.Written += int64(n)

	if time.Since(wc.LastUpdate) >= 100*time.Millisecond {
		_, err = fmt.Fprintf(log.Writer(), "\rDownloading... %s transferred %c \033[K", byteCountToHumanReadable(wc.Written), wc.animate())
		if err != nil {
			log.Printf("error writing progress: %v", err)
		}
		wc.LastUpdate = time.Now()
	}

	return n, nil
}

// byteCountToHumanReadable converts a byte count to a human-readable format (e.g., KB, MB, GB).
func byteCountToHumanReadable(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (wc *WriteCounter) animate() byte {
	anim := "/-\\|"
	wc.currentAnim = (wc.currentAnim + 1) % len(anim)
	return anim[wc.currentAnim]
}
