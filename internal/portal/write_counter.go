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
	Total       int64
	StartBytes  int64
	LastUpdate  time.Time
	StartTime   time.Time
	Writer      io.Writer
	currentAnim int
}

// NewWriteCounter creates a new WriteCounter.
func NewWriteCounter(writer io.Writer) *WriteCounter {
	return &WriteCounter{
		Writer: writer,
		// Initialize to zero so the first Write triggers an immediate log
		LastUpdate: time.Time{},
		StartTime:  time.Now(),
	}
}

// NewWriteCounterWithTotal creates a new WriteCounter with known total size.
func NewWriteCounterWithTotal(writer io.Writer, total int64, startBytes int64) *WriteCounter {
	return &WriteCounter{
		Writer:     writer,
		Total:      total,
		StartBytes: startBytes,
		LastUpdate: time.Time{},
		StartTime:  time.Now(),
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
		var progress string
		if wc.Total > 0 {
			currentTotal := wc.StartBytes + wc.Written
			percentage := float64(currentTotal) / float64(wc.Total) * 100
			elapsed := time.Since(wc.StartTime)
			speed := float64(wc.Written) / elapsed.Seconds()

			var eta string
			if speed > 0 {
				remaining := wc.Total - currentTotal
				etaSeconds := float64(remaining) / speed
				eta = FormatDuration(time.Duration(etaSeconds) * time.Second)
			} else {
				eta = "calculating..."
			}

			progress = fmt.Sprintf("\rDownloading... %.1f%% (%s / %s) | Speed: %s/s | ETA: %s %c \033[K",
				percentage,
				ByteCountToHumanReadable(currentTotal),
				ByteCountToHumanReadable(wc.Total),
				ByteCountToHumanReadable(int64(speed)),
				eta,
				wc.animate())
		} else {
			elapsed := time.Since(wc.StartTime)
			speed := float64(wc.Written) / elapsed.Seconds()
			progress = fmt.Sprintf("\rDownloading... %s | Speed: %s/s %c \033[K",
				ByteCountToHumanReadable(wc.Written),
				ByteCountToHumanReadable(int64(speed)),
				wc.animate())
		}

		_, err = fmt.Fprint(log.Writer(), progress)
		if err != nil {
			log.Printf("error writing progress: %v", err)
		}
		wc.LastUpdate = time.Now()
	}

	return n, nil
}

// ByteCountToHumanReadable converts a byte count to a human-readable format (e.g., KB, MB, GB).
func ByteCountToHumanReadable(b int64) string {
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

// FormatDuration formats a duration in a human-readable format.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, minutes)
}
