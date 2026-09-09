# Lil

Lil is a self-hosted URL shortener written in Go. It stores links in SQLite
and can send iOS, Android, and desktop clicks to different destinations.

Lil includes:

- transactional SQLite writes and an in-memory redirect cache
- custom slugs, expiry times, titles, and platform destinations
- a Vue admin UI for creating, editing, and deleting links
- Plausible, access-log, and webhook analytics providers
- Prometheus metrics
- a JSON management API

![Create a short link](docs/screenshots/2.png)

![Dashboard in light mode](docs/screenshots/3.png)

![Dashboard in dark mode](docs/screenshots/4.png)

## Start a local server

You need Go 1.26 or newer, Node 22.12 or newer, pnpm 11.3.0, and tmux.
Use Node 24 when possible.

```sh
make dev
```

Open <http://localhost:5173/admin/>. The API and short links use
<http://localhost:17000>. Local data stays in `.dev/urls.db`. The local
configuration disables authentication and analytics.

The development server binds to loopback. It does not expose the admin UI or
database to other computers on your network.

Useful commands:

| Command | Purpose |
|---|---|
| `make dev-logs` | Show recent API and UI logs |
| `make dev-attach` | Attach to both tmux windows |
| `make dev-restart` | Rebuild and restart after a Go change |
| `make dev-stop` | Stop the development server |
| `make check` | Run tests, static analysis, type checks, and lint checks |
| `make clean` | Clean build output without deleting the local database |

Vite reloads frontend changes. Run `make dev-restart` after a Go change.

## Configure Lil

Copy the sample before you run a release binary:

```sh
cp config.sample.toml config.toml
```

At minimum, review these settings:

```toml
[server]
address = ":7000"

[db]
path = "urls.db"

[app]
short_url_length = 6
public_url = "https://links.example.com"

[admin]
username = "admin"
password = "replace-this-password"

[analytics]
enabled = false
num_workers = 2
```

`app.public_url` is the base URL that Lil shows and copies in the admin UI.
Set both admin credentials to protect the admin UI, management API, and
metrics endpoint. If both values are empty, Lil disables Basic Auth. Do not
run that configuration on a public network.

See [`config.sample.toml`](config.sample.toml) for the analytics provider
settings. When you enable analytics, Lil starts every provider table in the
configuration unless that table contains `enabled = false`. Delete provider
tables that you do not use.

## Route by platform

A link can store four destinations:

- the original URL, which is always required
- an Android URL
- an iOS URL
- a Web URL

Clients should normally share the plain short link, such as
`https://links.example.com/rate-us`. Lil selects a platform for each click.
It uses this order:

1. Use `?platform=android`, `?platform=ios`, or `?platform=web` when present.
2. Send recognized crawlers to Web.
3. Use a quoted `Sec-CH-UA-Platform` client hint when available.
4. Parse the User-Agent header.
5. Use Web when the client remains unknown.

If the selected platform has no destination, Lil uses the original URL. It
does not append query parameters from the short link to the destination.
Queries saved in a destination, such as `?action=write-review`, stay intact.

The platform query parameter is an override for testing and callers that
already know the device. Email and website links should usually omit it.
Lil rejects empty, repeated, and unknown platform values with HTTP 400.

An iPad in desktop mode can send the same `Macintosh` User-Agent as a Mac.
Lil treats this request as Web because the server cannot distinguish the two.
Use a store-choice page as the Web destination when this case matters.

Redirects return HTTP 302 with these headers:

```text
Cache-Control: private, no-store
Vary: User-Agent
Vary: Sec-CH-UA-Platform
```

Your proxy or CDN must respect these headers. The browser and operating
system decide whether a store URL opens an app or a web page.

Each successful redirect writes a structured `redirect selected` log entry.
The entry includes the short code, platform, detection source, destination
host, destination query keys, User-Agent, Cloudflare Ray ID, and a SHA-256 hash
of the full destination. Lil does not log destination query values.

## Run with Docker Compose

The included Compose file starts Lil on loopback and stores the database in a
named volume.

```sh
docker compose up -d
```

Open <http://localhost:7000/admin/>. This configuration is for local testing.
It has no admin password and enables only access-log analytics.

For a production deployment, provide your own `config.toml`, set admin
credentials, use a reverse proxy for TLS, and restrict `/admin/` and
`/api/` as needed. Run one Lil process for each SQLite database. Separate
processes do not share redirect-cache updates.

## Run a release binary

Download an archive from [GitHub Releases](https://github.com/mr-karan/lil/releases),
extract it, and run:

```sh
./lil.bin --config=config.toml
```

The binary embeds the admin UI. Open `/admin/` on the configured server
address.

## API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/{shortCode}` | Redirect a public short link |
| `GET` | `/api/v1/health` | Check SQLite connectivity |
| `GET` | `/api/v1` | Read the version and public URL |
| `POST` | `/api/v1/shorten` | Create a link |
| `GET` | `/api/v1/urls` | List links |
| `PUT` | `/api/v1/urls/{shortCode}` | Update a link |
| `DELETE` | `/api/v1/urls/{shortCode}` | Delete a link |
| `GET` | `/api/v1/metrics` | Read Prometheus metrics |

The health endpoint and short links are public. If you set admin credentials,
Lil applies Basic Auth to the other API routes. POST and PUT requests require
`Content-Type: application/json`. Lil limits request bodies to 1 MiB.

See [`docs/api.md`](docs/api.md) for request and response examples.

## Storage and shutdown

SQLite is the source of truth. Lil commits a link and its platform
destinations in one transaction. It updates the redirect cache only after the
commit succeeds.

Lil uses WAL mode and synchronous commits. It handles `SIGINT` and `SIGTERM`,
stops accepting requests, waits for analytics workers, and then closes the
database.

## License

See [`LICENSE`](LICENSE).

## Contributing

Bug reports and focused pull requests are welcome.
