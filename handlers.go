package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mr-karan/lil/internal/analytics"
	"github.com/mr-karan/lil/internal/metrics"
	"github.com/mr-karan/lil/internal/redirect"
	"github.com/mr-karan/lil/internal/store"
)

type shortenURLRequest struct {
	URL          string            `json:"url"`
	Title        string            `json:"title,omitempty"`
	Slug         string            `json:"slug,omitempty"`
	ExpiryInSecs *int64            `json:"expiry_in_secs,omitempty"`
	DeviceURLs   map[string]string `json:"device_urls,omitempty"` // platform -> url mapping
}

var slugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

func decodeJSON(r *http.Request, value any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

func (req shortenURLRequest) validate() error {
	if err := redirect.ValidateDestinations(req.URL, req.DeviceURLs); err != nil {
		return err
	}
	if req.Slug != "" {
		if !slugPattern.MatchString(req.Slug) || req.Slug == "admin" || req.Slug == "api" {
			return fmt.Errorf("slug must use letters, digits, underscores or hyphens and cannot be admin or api")
		}
	}
	if req.ExpiryInSecs != nil && (*req.ExpiryInSecs < 0 || *req.ExpiryInSecs > int64((time.Duration(1<<63-1))/time.Second)) {
		return fmt.Errorf("expiry is outside the supported range")
	}
	return nil
}

// httpResp represents the structure of the JSON response envelope
type httpResp struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// sendResponse sends a JSON envelope to the HTTP response.
func (app *App) sendResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	out, err := json.Marshal(httpResp{Status: "success", Data: data})
	if err != nil {
		app.sendErrorResponse(w, "Internal Server Error.", http.StatusInternalServerError, nil)
		return
	}
	w.Write(out)
}

// sendErrorResponse sends an error response to the HTTP response.
func (app *App) sendErrorResponse(w http.ResponseWriter, message string, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	out, err := json.Marshal(httpResp{Status: "error", Message: message, Data: data})
	if err != nil {
		app.logger.Error("Failed to marshal error response", "error", err)
		return
	}
	w.Write(out)
}

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	app.sendResponse(w, map[string]interface{}{
		"version":    buildString,
		"public_url": ko.String("app.public_url"),
	})
}

func (app *App) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if err := app.store.Ping(r.Context()); err != nil {
		app.sendErrorResponse(w, "Database is not healthy", http.StatusServiceUnavailable, nil)
		return
	}
	app.sendResponse(w, "healthy")
}

func (app *App) handleShortenURL(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req shortenURLRequest
	if err := decodeJSON(r, &req); err != nil {
		app.logger.Error("Invalid request body", "error", err)
		app.sendErrorResponse(w, "Invalid request body", http.StatusBadRequest, nil)
		return
	}

	// Basic validation
	if err := req.validate(); err != nil {
		app.sendErrorResponse(w, err.Error(), http.StatusBadRequest, nil)
		return
	}

	// Calculate expiry time if provided
	var expiry time.Duration
	if req.ExpiryInSecs != nil && *req.ExpiryInSecs > 0 {
		expiry = time.Duration(*req.ExpiryInSecs) * time.Second
	}

	// Call store method to create short URL with device URLs
	shortCode, err := app.store.CreateShortURL(r.Context(), req.URL, req.Title, req.Slug, expiry, req.DeviceURLs)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			app.sendErrorResponse(w, err.Error(), http.StatusConflict, nil)
			return
		}
		app.logger.Error("Failed to create short URL", "error", err, "url", req.URL)
		app.sendErrorResponse(w, "Failed to create short URL", http.StatusInternalServerError, nil)
		return
	}

	// Return the shortened URL with public base URL
	metrics.URLsShortenedTotal.Inc()
	app.sendResponse(w, map[string]interface{}{
		"short_code": shortCode,
		"public_url": ko.String("app.public_url"),
	})
}

func (app *App) handleRedirect(w http.ResponseWriter, r *http.Request) {
	// Extract shortCode from path
	shortCode := r.PathValue("shortCode")
	if shortCode == "" {
		app.sendErrorResponse(w, "Invalid short code", http.StatusBadRequest, nil)
		return
	}

	// Get URL data from store
	urlData, err := app.store.GetRedirectData(r.Context(), shortCode)
	if err != nil {
		if err == store.ErrNotExist {
			metrics.RedirectFailuresTotal.Inc()
			app.sendErrorResponse(w, "URL not found", http.StatusNotFound, nil)
			return
		}
		app.logger.Error("Failed to get URL data", "error", err, "shortCode", shortCode)
		app.sendErrorResponse(w, "Internal server error", http.StatusInternalServerError, nil)
		return
	}

	platform, err := redirect.Detect(r)
	if err != nil {
		app.sendErrorResponse(w, err.Error(), http.StatusBadRequest, nil)
		return
	}
	targetURL := redirect.Destination(urlData, platform)

	metrics.RedirectsTotal.Inc()
	if app.analytics != nil {
		// Extract real IP address from headers
		var userIP string
		if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
			userIP = cfIP
		} else if fwdIP := r.Header.Get("X-Forwarded-For"); fwdIP != "" {
			// Use the first IP in the chain which is typically the original client
			if firstIP := strings.Split(fwdIP, ",")[0]; firstIP != "" {
				userIP = strings.TrimSpace(firstIP)
			}
		} else {
			userIP = r.RemoteAddr
		}

		app.analytics.Track(analytics.Event{
			Name:       "pageview",
			Domain:     r.Host,
			URL:        fmt.Sprintf("%s/%s", ko.String("app.public_url"), shortCode),
			Referrer:   r.Header.Get("Referer"),
			UserAgent:  r.UserAgent(),
			UserIP:     userIP,
			RemoteAddr: r.RemoteAddr,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			ShortCode:  shortCode,
			TargetURL:  targetURL,
		})
	}

	// Ensure browsers don't cache the redirect response
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Add("Vary", "User-Agent")
	w.Header().Add("Vary", "Sec-CH-UA-Platform")
	w.Header().Set("X-Lil-Platform", string(platform))
	w.Header().Set("Location", targetURL)
	w.WriteHeader(http.StatusFound)
}

func (app *App) handleGetURLs(w http.ResponseWriter, r *http.Request) {
	// Get pagination parameters from query string
	page := r.URL.Query().Get("page")
	perPage := r.URL.Query().Get("per_page")

	// Convert to int64 with defaults
	pageNum := int64(1)
	if page != "" {
		if p, err := strconv.ParseInt(page, 10, 64); err == nil {
			pageNum = p
		}
	}

	perPageNum := int64(10)
	if perPage != "" {
		if pp, err := strconv.ParseInt(perPage, 10, 64); err == nil {
			perPageNum = pp
		}
	}

	// Fetch URLs from store
	if pageNum < 1 || pageNum > 1000000 || perPageNum < 1 || perPageNum > 1000 {
		app.sendErrorResponse(w, "page must be 1..1000000 and per_page 1..1000", http.StatusBadRequest, nil)
		return
	}
	urls, total, err := app.store.GetURLs(r.Context(), pageNum, perPageNum)
	if err != nil {
		app.logger.Error("Failed to fetch URLs", "error", err)
		app.sendErrorResponse(w, "Failed to fetch URLs", http.StatusInternalServerError, nil)
		return
	}

	// Return the URLs
	app.sendResponse(w, map[string]interface{}{
		"urls":     urls,
		"page":     pageNum,
		"per_page": perPageNum,
		"count":    total,
	})
}

func (app *App) handleUpdateURL(w http.ResponseWriter, r *http.Request) {
	// Extract shortCode from path
	shortCode := r.PathValue("shortCode")
	if shortCode == "" {
		app.sendErrorResponse(w, "Invalid short code", http.StatusBadRequest, nil)
		return
	}

	// Parse request body
	var req shortenURLRequest
	if err := decodeJSON(r, &req); err != nil {
		app.logger.Error("Invalid request body", "error", err)
		app.sendErrorResponse(w, "Invalid request body", http.StatusBadRequest, nil)
		return
	}

	// Basic validation
	if err := req.validate(); err != nil {
		app.sendErrorResponse(w, err.Error(), http.StatusBadRequest, nil)
		return
	}

	// Update URL in store
	if err := app.store.UpdateURL(r.Context(), shortCode, req.URL, req.Title, req.DeviceURLs); err != nil {
		if err == store.ErrNotExist {
			app.sendErrorResponse(w, "URL not found", http.StatusNotFound, nil)
			return
		}
		app.logger.Error("Failed to update URL", "error", err, "shortCode", shortCode)
		app.sendErrorResponse(w, "Internal server error", http.StatusInternalServerError, nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *App) handleDeleteURL(w http.ResponseWriter, r *http.Request) {
	// Extract shortCode from path
	shortCode := r.PathValue("shortCode")
	if shortCode == "" {
		app.sendErrorResponse(w, "Invalid short code", http.StatusBadRequest, nil)
		return
	}

	// Delete URL from store
	if err := app.store.DeleteURL(r.Context(), shortCode); err != nil {
		if err == store.ErrNotExist {
			app.sendErrorResponse(w, "URL not found", http.StatusNotFound, nil)
			return
		}
		app.logger.Error("Failed to delete URL", "error", err, "shortCode", shortCode)
		app.sendErrorResponse(w, "Internal server error", http.StatusInternalServerError, nil)
		return
	}

	// Return success with no content
	metrics.URLsDeletedTotal.Inc()
	w.WriteHeader(http.StatusNoContent)
}
