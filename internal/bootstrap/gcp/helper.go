package gcp

import (
	"fmt"
	"time"
)

// calculateProjectExpiryLabel takes a TTL string (e.g. "24h") and
// returns a formatted UTC timestamp string that is usable as a GCP project label for automatic deletion.
func calculateProjectExpiryLabel(projectTTLStr string) (string, error) {
	projectTTL, err := time.ParseDuration(projectTTLStr)
	if err != nil {
		return "", fmt.Errorf("invalid project TTL format: %w", err)
	}

	// prepare label for gcp project deletion in custom UTC time format.
	// GCP Labels are very limited. This is an easy way to add date and TZ info in one label.
	gcpExpiryLabelLayout := "2006-01-02_15-04-05"
	deleteProjectAfter := time.Now().UTC().Add(projectTTL).Format(gcpExpiryLabelLayout)
	deleteProjectAfter = fmt.Sprintf("%s_utc", deleteProjectAfter)

	return deleteProjectAfter, nil
}
