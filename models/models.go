package models

import "time"

type UserRef struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type URLData struct {
	URL        string                   `json:"url"`
	Title      string                   `json:"title,omitempty"`
	ShortCode  string                   `json:"short_code"`
	CreatedAt  time.Time                `json:"created_at"`
	ExpiresAt  *time.Time               `json:"expires_at"`
	CreatedBy  *UserRef                 `json:"created_by"`
	UpdatedBy  *UserRef                 `json:"updated_by"`
	UpdatedAt  *time.Time               `json:"updated_at"`
	DeviceURLs map[string]DeviceURLData `json:"device_urls,omitempty"`
}

type DeviceURLData struct {
	URL       string    `json:"url"`
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"created_at"`
}
