package portal

import "time"

type ApiKey struct {
	KeyID        string    `json:"key_id"`
	Owner        string    `json:"owner"`
	Organization string    `json:"organization"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	// Temp
	ApiKey string `json:"api_key"`
}
