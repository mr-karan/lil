# Lil - High-Performance URL Shortener

Lil is a fast URL shortener built in Go, designed with scalability and extensibility in mind.

## Key Features

- **Storage**: Transactional SQLite writes with an in-memory redirect cache
- **Platform-Specific Redirects**: Intelligently route users based on their device
  - Custom redirects for iOS, Android, and web.
  - Fallback URLs for unsupported platforms
  - Device detection using User-Agent headers
- **Flexible Analytics**: Supports multiple analytics providers out of the box
  - Plausible Analytics integration
  - Access log provider for analysis with tools like GoAccess
  - Custom webhook support for easy integration with other services
- **Admin UI**: Clean, responsive dashboard built with Vue.js
- **Monitoring**: Built-in Prometheus metrics for observability
- **URL Management**:
  - Custom slugs support
  - URL expiration
  - Title and metadata storage
  - Pagination and search in admin UI

## Architecture Overview

- **Storage**: SQLite is the source of truth. Creates and edits commit the URL and device destinations together before updating the in-memory redirect cache.
- **Async Analytics**: Background workers handle analytics dispatch without impacting redirect performance
- **Extensible**: Easy to add new analytics providers through a simple interface
- **API**: RESTful JSON API for programmatic access
- **Metrics**: Prometheus metrics for redirects, failures, mutations, and stored URLs.

---

![Dashboard](docs/screenshots/2.png)
*Create URL*

![Dashboard Light](docs/screenshots/3.png)
*Dashboard*

![Dashboard Dark](docs/screenshots/4.png)
*Dark Mode*

---

## Getting Started

### Local development

Requirements: Go 1.26+, Node 22.12+ (Node 24 recommended), pnpm 11.3.0,
and tmux. Run commands from the repository root.

```sh
make dev          # Build, then start API and hot-reloading Vue UI
make dev-logs     # Recent logs
make dev-attach   # Live terminals; Ctrl-b d detaches
make dev-restart  # Rebuild the backend after Go changes
make dev-stop     # Stop the dev processes
make check       # Race tests, vet, staticcheck, Vue type checks, ESLint
```

Open **http://localhost:5173/admin/**. Local development has no login and
binds to loopback. Redirects and the API use **http://localhost:17000**.
Data lives in `.dev/urls.db`, independently of `config.toml` and `urls.db`.
Analytics is disabled in `dev/local.toml`. Frontend edits hot reload;
Go edits require `make dev-restart`. `make clean` preserves the dev database.

For a production-style local build, use `make run CONFIG=dev/local.toml`
after stopping the dev API. The embedded UI is at port 17000 `/admin/`.

### Platform redirects

The server selects the platform in this order:

1. Explicit `?platform=android`, `?platform=ios`, or `?platform=web`.
   Empty, repeated, and unsupported overrides return 400.
2. Recognized crawler User-Agents use Web, regardless of client hints.
3. A recognized, quoted `Sec-CH-UA-Platform` client hint.
4. User-Agent parsing, then Web for unknown or missing device information.

The selected device destination overrides the original URL. If that device
destination is absent or empty, the original URL is used. Request query
parameters are not appended to destinations; saved destination queries such
as `action=write-review` are preserved exactly.

An iPad in desktop mode can be indistinguishable from a Mac. The server does
not guess based on `Macintosh`. Use an explicit platform link when the caller
knows the device, or make Web a page with App Store and Play Store buttons.
An override chooses a saved destination only; it cannot supply a new URL.

Redirect responses use HTTP 302, `Cache-Control: private, no-store`, and
`Vary: User-Agent, Sec-CH-UA-Platform`. Configure proxies/CDNs to respect those
headers. Browser and OS policies still determine whether a store app opens;
a redirect cannot guarantee a native review prompt.

### Upgrade notes

- Go and npm dependencies are updated and locked. Vite 8, Tailwind 4, and
  daisyUI 5 require modern browsers. TypeScript stays on 6.0.3 because the
  current vue-tsc release does not support TypeScript 7's compiler layout.
- Existing SQLite migrations and tables are preserved. The old write buffer,
  connection pool and flush settings are no longer used. SQLite uses one
  persistent connection with WAL and synchronous commits. Redirect reads remain
  in memory, while acknowledged writes are durable and immediately update the
  cache.
- Basic Auth now protects management APIs as well as the admin UI when both
  admin credentials are configured. API clients and metrics scrapers must
  send credentials. The health endpoint and public short links remain public.
- `/admin` redirects to `/admin/`. Use the trailing slash for the portal.
- Create and update reject invalid destinations and unknown device keys.
  Only absolute HTTP(S) destinations without embedded credentials are accepted.
- The dashboard preserves Web overrides and copies links using `app.public_url`.

### Using Docker

The easiest way to run Lil is using Docker:

```bash
docker run -p 7000:7000 \
  -v $(pwd)/config.toml:/app/config.toml \
  -v $(pwd)/urls.db:/app/urls.db \
  ghcr.io/mr-karan/lil:latest
```

### Using Docker Compose

A complete example with persistent storage:

```bash
docker compose up -d
```

The sample Compose setup binds to loopback and is for local evaluation. Supply
an authenticated configuration and a production ingress for deployment.

### Manual Installation

1. Download the latest release
2. Configure via `config.toml`
3. Run the binary
4. Access admin UI at `/admin`

## Configuration

See `config.toml` for all available options. Key sections:

```toml
[server]
address = ":7000"

[db]
path = "urls.db"

[analytics]
enabled = true
num_workers = 2

[analytics.providers.plausible]
endpoint = "http://plausible:8000/api/event"
```

## API Documentation

See `docs/api.md` for detailed API documentation.

## Domain Setup

Lil supports running on separate domains for public URL shortening and admin interface:

### Configuration

1. Set your public domain in `config.toml`:
```toml
[app]
public_url = "https://lil.io"  # Base URL for shortened URLs
```

### Architecture

The application can be deployed with two separate domains. For eg the following config can be referred for production deployments:

### Nginx Configuration Example

```ini
# Public URL shortener
server {
    listen 80;
    server_name lil.io;

    # Block access to admin interface and API
    location ~ ^/(admin|api) {
        return 403;
    }

    # Forward everything else to the application
    location / {
        proxy_pass http://localhost:7000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}

# Admin interface
server {
    listen 80;
    server_name liladmin.internal;

    # Only allow internal network access
    allow 10.0.0.0/8;
    allow 172.16.0.0/12;
    allow 192.168.0.0/16;
    deny all;

    location / {
        proxy_pass http://localhost:7000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## License

[LICENSE](./LICENSE)

## Contributing

Contributions welcome! Please read our contributing guidelines before submitting PRs.
