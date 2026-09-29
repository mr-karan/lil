package auth

import (
	"net/url"
	"path"
	"strings"
)

const defaultNext = "/admin/"

// sanitizeNext returns raw only when it is a clean same-site path under /admin.
func sanitizeNext(raw string) string {
	if strings.Contains(raw, `\`) {
		return defaultNext
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return defaultNext
	}
	escaped := strings.ToLower(u.EscapedPath())
	for _, encoded := range []string{"%2e", "%2f", "%5c"} {
		if strings.Contains(escaped, encoded) {
			return defaultNext
		}
	}
	if path.Clean(u.Path) != strings.TrimSuffix(u.Path, "/") {
		return defaultNext
	}
	if u.Path != "/admin" && !strings.HasPrefix(u.Path, "/admin/") {
		return defaultNext
	}
	next := u.EscapedPath()
	if u.RawQuery != "" {
		next += "?" + u.RawQuery
	}
	return next
}
