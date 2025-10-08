package portal

import "time"

type ApiKey struct {
	RID          string    `json:"rid"`
	ApiKey       string    `json:"api_key"`
	Owner        string    `json:"owner"`
	Organization string    `json:"organization"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
