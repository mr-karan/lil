# URL Shortener API Documentation

## Authentication

Every endpoint except `GET /api/v1/health` and `GET /{shortCode}` needs
authentication. Send a per-user API token:

```
Authorization: Bearer lil_<secret>
```

Create tokens in the admin UI or with `POST /api/v1/tokens`. A request that
carries an `Authorization` header uses only that token. Basic Auth is not
supported. Without valid credentials, Lil answers HTTP 401:

```json
{
  "status": "error",
  "message": "authentication required"
}
```

Browsers use a session cookie from the OIDC sign-in. Browser writes from another
origin get HTTP 403. Requests without `Origin` or `Sec-Fetch-Site` headers, such
as `curl`, are not affected.

JSON responses use the envelope `{"status": "success", "data": ...}`.
Endpoints that answer HTTP 204 have no body.

## Shorten URL

Create a shortened URL from a long URL.

**Endpoint:** `POST /api/v1/shorten`

**Request Body:**
```json
{
  "url": "https://example.com/very/long/url",  // Required
  "title": "My Link",                          // Optional
  "slug": "custom-slug",                       // Optional, custom short code
  "expiry_in_secs": 3600                      // Optional, URL expiry in seconds
}
```

**Response:**
```json
{
  "status": "success",
  "data": {
    "short_code": "abc123",
    "public_url": "https://lil.io"
  }
}
```

**Error Response:**
```json
{
  "status": "error",
  "message": "Error message here"
}
```

## Get URLs

Retrieve a paginated list of shortened URLs.

**Endpoint:** `GET /api/v1/urls`

**Query Parameters:**
- `page`: Page number (default: 1)
- `per_page`: Items per page (default: 10)

**Response:**
```json
{
  "status": "success",
  "data": {
    "urls": [
      {
        "url": "https://example.com/long/url",
        "title": "My Link",
        "short_code": "abc123",
        "created_at": "2024-01-01T00:00:00Z",
        "expires_at": "2024-01-02T00:00:00Z",
        "created_by": {"id": 1, "email": "alice@example.com", "name": "Alice"},
        "updated_by": {"id": 2, "email": "bob@example.com", "name": "Bob"},
        "updated_at": "2024-01-01T12:00:00Z"
      }
    ],
    "page": 1,
    "per_page": 10,
    "count": 1
  }
}
```

`created_by` and `updated_by` are `null` for links that predate user tracking
(`created_by`) or that nobody has edited (`updated_by`). `updated_at` is `null`
until the first edit.

## Update URL

**Endpoint:** `PUT /api/v1/urls/{shortCode}`

The body has the same fields as Shorten URL (`url` is required; `slug` and
`expiry_in_secs` are ignored). **Response:** HTTP 204 No Content. Lil sets
`updated_by` and `updated_at`.

## Delete URL

Delete a shortened URL.

**Endpoint:** `DELETE /api/v1/urls/{shortCode}`

**Response:** HTTP 204 No Content

**Error Response:**
```json
{
  "status": "error",
  "message": "URL not found"
}
```

## Health Check

Check if the service is healthy.

**Endpoint:** `GET /api/v1/health`

**Response:**
```json
{
  "status": "success",
  "data": "healthy"
}
```

## Redirect

Redirect to the original URL.

**Endpoint:** `GET /{shortCode}`

**Response:** HTTP 302 Found with Location header

**Error Response:**
```json
{
  "status": "error",
  "message": "URL not found"
}
```

## Current user

**Endpoint:** `GET /api/v1/me`

**Response:**
```json
{
  "status": "success",
  "data": {
    "id": 1,
    "email": "alice@example.com",
    "name": "Alice",
    "created_at": "2024-01-01T00:00:00Z",
    "last_login_at": "2024-01-05T09:00:00Z",
    "disabled_at": null
  }
}
```

## Users

**List:** `GET /api/v1/users` returns an array of users (same shape as
`/api/v1/me`), ordered by email.

**Disable:** `POST /api/v1/users/{id}/disable` returns HTTP 204. The user's
sessions stop working and Lil revokes all their API tokens. HTTP 400
(`you cannot disable yourself`) for your own id. HTTP 404 for an unknown id.

**Enable:** `POST /api/v1/users/{id}/enable` returns HTTP 204. Enabling does
not restore revoked tokens or ended sessions.

These calls take no request body.

## API tokens

Tokens belong to the caller and carry full admin power.

**List:** `GET /api/v1/tokens` returns the caller's active tokens, newest first:
```json
{
  "status": "success",
  "data": [
    {"id": 3, "name": "ci", "token_prefix": "lil_AbCdEfGh", "created_at": "2024-01-01T00:00:00Z"}
  ]
}
```

**Create:** `POST /api/v1/tokens` with `{"name": "ci"}` (1 to 64 characters after
trimming). Lil returns the plaintext token once:
```json
{
  "status": "success",
  "data": {
    "token": "lil_...",
    "api_token": {"id": 3, "name": "ci", "token_prefix": "lil_AbCdEfGh", "created_at": "2024-01-01T00:00:00Z"}
  }
}
```

**Revoke:** `DELETE /api/v1/tokens/{id}` returns HTTP 204. HTTP 404 if the token
does not exist, is already revoked, or belongs to someone else.

## Activity log

**Endpoint:** `GET /api/v1/audit`

**Query Parameters:**
- `short_code`: only entries for this link, including after it was deleted
- `page`: Page number (default: 1, max: 1000000)
- `per_page`: Items per page (default: 10, max: 1000)

**Response:**
```json
{
  "status": "success",
  "data": {
    "entries": [
      {
        "id": 12,
        "created_at": "2024-01-01T12:00:00Z",
        "actor": {"id": 2, "email": "bob@example.com", "name": "Bob"},
        "token_id": 3,
        "action": "url.update",
        "target": "abc123",
        "before": {"url": "https://example.com/a", "title": "", "expires_at": null, "device_urls": {}},
        "after": {"url": "https://example.com/b", "title": "", "expires_at": null, "device_urls": {}}
      }
    ],
    "page": 1,
    "per_page": 10,
    "count": 1
  }
}
```

`action` is one of `url.create`, `url.update`, `url.delete`, `token.create`,
`token.revoke`, `user.disable`, `user.enable`. `target` is a short code, a token
id, or a user id. `token_id` is `null` for browser sessions. `before` and
`after` are `null` when they do not apply. Entries are newest first. Automatic
expiry deletions are not logged.
