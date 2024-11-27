package models

import "time"

type URLData struct {
	URL        string                   `json:"url"`
	Title      string                   `json:"title,omitempty"`
	ShortCode  string                   `json:"short_code"`
	CreatedAt  time.Time                `json:"created_at"`
	ExpiresAt  *time.Time               `json:"expires_at"`
	DeviceURLs map[string]DeviceURLData `json:"device_urls,omitempty"`
}

type DeviceURLData struct {
	URL       string    `json:"url"`
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"created_at"`
}
