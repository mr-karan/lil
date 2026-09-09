package redirect

import (
	"net/http/httptest"
	"testing"

	"github.com/mr-karan/lil/models"
)

func TestDetect(t *testing.T) {
	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_3_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Mobile/15E148 Safari/604.1"
	android := "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Mobile Safari/537.36"
	for _, tt := range []struct {
		name, ua, hint, query string
		want                  Platform
		invalid               bool
	}{
		{"iphone safari", iphone, "", "", IOS, false},
		{"iphone chrome", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 CriOS/121.0.0.0 Mobile/15E148 Safari/604.1", "", "", IOS, false},
		{"ipad mobile", "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1", "", "", IOS, false},
		{"ipad desktop ambiguous", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15", "", "", Web, false},
		{"android chrome", android, "", "", Android, false},
		{"android webview", "Mozilla/5.0 (Linux; Android 13; Pixel 7 Build/TQ3A; wv) AppleWebKit/537.36 Version/4.0 Chrome/120.0.0.0 Mobile Safari/537.36", "", "", Android, false},
		{"ios webview", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148", "", "", IOS, false},
		{"android hint", "", `"Android"`, "", Android, false},
		{"desktop hint takes precedence", iphone, `"macOS"`, "", Web, false},
		{"malformed hint falls back", iphone, "Android", "", IOS, false},
		{"explicit ios", android, `"Android"`, "?platform=ios", IOS, false},
		{"explicit web", iphone, "", "?platform=web", Web, false},
		{"unknown override", iphone, "", "?platform=windows", "", true},
		{"duplicate override", iphone, "", "?platform=ios&platform=android", "", true},
		{"empty override", iphone, "", "?platform=", "", true},
		{"missing UA", "", "", "", Web, false},
		{"unknown UA", "ExampleApp/3", "", "", Web, false},
		{"mobile bot", "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/41.0.2272.96 Mobile Safari/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "", "", Web, false},
		{"mobile bot with hint", "Mozilla/5.0 (Linux; Android 6.0.1) AppleWebKit/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", `"Android"`, "", Web, false},
		{"explicit override for bot", "Googlebot/2.1", `"Android"`, "?platform=ios", IOS, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/rate-us"+tt.query, nil)
			r.Header.Set("User-Agent", tt.ua)
			r.Header.Set("Sec-CH-UA-Platform", tt.hint)
			got, err := Detect(r)
			if (err != nil) != tt.invalid || got != tt.want {
				t.Fatalf("got %q, %v; want %q, invalid=%v", got, err, tt.want, tt.invalid)
			}
		})
	}
}

func TestDestination(t *testing.T) {
	data := models.URLData{URL: "https://example.com/fallback", DeviceURLs: map[string]models.DeviceURLData{
		"ios": {URL: "https://apps.apple.com/app/id123456789?action=write-review"},
		"web": {URL: "https://example.com/choose"},
	}}
	if got := Destination(data, IOS); got != data.DeviceURLs["ios"].URL {
		t.Fatal(got)
	}
	if got := Destination(data, Web); got != data.DeviceURLs["web"].URL {
		t.Fatal(got)
	}
	if got := Destination(data, Android); got != data.URL {
		t.Fatal(got)
	}
}

func TestValidateURL(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "//example.com", "/relative", "https://user:pass@example.com", "https://example.com\r\nLocation: evil", ""} {
		if ValidateURL(raw) == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	if err := ValidateURL("https://apps.apple.com/app/id123456789?action=write-review"); err != nil {
		t.Fatal(err)
	}
}
