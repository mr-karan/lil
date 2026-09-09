// Package redirect selects configured destinations without fetching them.
package redirect

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mileusna/useragent"
	"github.com/mr-karan/lil/models"
)

type Platform string

const (
	Android Platform = "android"
	IOS     Platform = "ios"
	Web     Platform = "web"
)

// Detect uses an explicit override first, then rejects bots, then uses client
// hints and the UA for human traffic.
// An indistinguishable desktop-mode iPad is deliberately treated as web.
func Detect(r *http.Request) (Platform, error) {
	if values, present := r.URL.Query()["platform"]; present {
		if len(values) != 1 {
			return "", fmt.Errorf("provide exactly one platform")
		}
		switch Platform(values[0]) {
		case Android, IOS, Web:
			return Platform(values[0]), nil
		default:
			return "", fmt.Errorf("platform must be android, ios, or web")
		}
	}
	ua := useragent.Parse(r.UserAgent())
	if ua.Bot {
		return Web, nil
	}
	switch r.Header.Get("Sec-CH-UA-Platform") {
	case `"Android"`:
		return Android, nil
	case `"iOS"`:
		return IOS, nil
	case `"Windows"`, `"macOS"`, `"Linux"`, `"Chrome OS"`:
		return Web, nil
	}
	switch {
	case ua.IsAndroid():
		return Android, nil
	case ua.IsIOS():
		return IOS, nil
	default:
		return Web, nil
	}
}

// Destination falls back to the original URL when an override is absent.
func Destination(data models.URLData, platform Platform) string {
	if target, ok := data.DeviceURLs[string(platform)]; ok && target.URL != "" {
		return target.URL
	}
	return data.URL
}

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || strings.ContainsAny(raw, "\r\n\t") {
		return fmt.Errorf("destination must be an absolute HTTP or HTTPS URL without credentials")
	}
	return nil
}

func ValidateDestinations(original string, devices map[string]string) error {
	if err := ValidateURL(original); err != nil {
		return err
	}
	for platform, target := range devices {
		switch Platform(platform) {
		case Android, IOS, Web:
		default:
			return fmt.Errorf("unsupported platform %q", platform)
		}
		if target != "" {
			if err := ValidateURL(target); err != nil {
				return fmt.Errorf("%s: %w", platform, err)
			}
		}
	}
	return nil
}
