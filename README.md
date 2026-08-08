# Whereabouts

Whereabouts (internally "Where·What") is a self-hosted web app for keeping track of where your
belongings physically are. You create a tree of **storage locations** (house → room → shelf →
box, as deep as you like), file **items** into them with an optional note and photos, and later
find anything back by browsing the tree, filtering by location, or searching by name/note/location
path. It's a personal inventory, not a shopping or task list.

The project is a single Go binary: a REST API backed by your choice of MySQL/MariaDB, PostgreSQL
or SQLite, with the entire frontend embedded inside it via `go:embed`. There is no separate
frontend build step, no Node toolchain, and no CORS setup to worry about — one binary listens on
one port and serves both the API and the UI.

## Highlights

- **Nested storage locations** — an unbounded tree of locations, each with a name and a color,
  rendered as a collapsible tree with per-location item counts (direct and total, including
  nested locations).
- **Items with photos** — a name, a location, a free-text note, and any number of photos. Photos
  are resized in the browser before upload and re-compressed defensively on the server, then
  stored as files on disk (not as base64 blobs in the database).
- **Search & filtering** — a text search across item name, note and full location path, plus
  location filter chips whose nesting depth is configurable per user.
- **Real, cookie-based authentication** — bcrypt-hashed passwords, random session tokens, httpOnly
  `SameSite=Lax` cookies, 30-day sessions, and an hourly background sweep of expired sessions.
  There is no public sign-up flow; accounts are created with a CLI subcommand.
- **A user profile**: change username/password (both re-confirm the current password), pick an
  interface language, and tune how many levels of location nesting show up as filter chips.
- **Five interface languages** — English, Russian, German, Spanish and French — for both the UI
  itself and the error messages the backend sends back, translated by exact string match on the
  frontend (see [Internationalization](#internationalization)).
- **Three interchangeable database backends** — MySQL/MariaDB, PostgreSQL or SQLite — selected by
  one environment variable, each with its own adapter, schema and idiomatic dialect.
- **Fail-fast configuration** — every setting is validated at startup; a missing or malformed
  environment variable stops the process with a clear message instead of silently doing the wrong
  thing.

## Screenshots / how it works

Open the app, log in (or use the seeded demo account), and you land on the **Items** tab: a
searchable, filterable list of everything you've filed away, each card showing its photo (or
initials), its location's breadcrumb path, and a relative "updated N ago" timestamp. The
**Locations** tab shows the same data the other way around — a tree you can expand, collapse, add
to, rename, recolor and prune. The **Profile** tab holds account settings.

## Architecture

The backend follows a hexagonal ("ports & adapters") architecture: business logic is organized as
one explicit **use case** per user action (e.g. `item/create`, `location/delete`,
`user/changepassword`), each with its own `Input`/`Output`/`Execute`. Use cases depend only on
interfaces declared in `internal/port` — repositories, a password hasher, a token generator, an
image processor, image storage — never on a concrete database driver or on `net/http`. Concrete
implementations of those interfaces ("adapters") are plugged in once, at the composition root.

```
webassets.go               go:embed for the frontend (must live at the module root — go:embed
                            can't reach outside the directory of the file that declares it)
cmd/whereabouts/
  main.go                  thin entry point; delegates everything to internal/app

internal/
  app/                     composition root: loads config, opens storage, wires every adapter and
                            use case together, starts the HTTP server, and the session-cleanup
                            goroutine. Also implements the `create-user` CLI subcommand.
  config/                  env var loading & validation (app.go / db.go / log.go), fails fast on
                            anything malformed
  domain/
    entity/                core types: Item, Location, User (+ PublicUser), Session
    error/                 typed errors — ValidationError, NotFoundError, ConflictError,
                            ForbiddenError, UnauthorizedError — mapped to HTTP status codes in one
                            place (internal/adapter/http/errors.go)
    usecase/                one directory per use case: item/, location/, auth/, user/
                            (create, update, delete, list, login, logout, authenticate,
                            changepassword, updateusername, updatelanguage,
                            updatelocationfilterdepth, …)
    service/
      passwordhasher/      bcrypt
      tokengenerator/      random session tokens
      imageprocessor/      dependency-free JPEG compression (see below)
  port/                    the interfaces use cases depend on (repositories, PasswordHasher,
                            TokenGenerator, ImageProcessor, ImageStorage), plus generated mocks
                            for testing
  repository/              one package per entity (item, location, session, user); translates
                            between domain entities and storage models, and turns
                            sql.ErrNoRows / unique-constraint violations into typed domain errors.
                            No SQL lives here — only calls into internal/storage.
  storage/
    model/                 DB row shapes, kept separate from domain entities (e.g. UpdatedAt as
                            the driver-formatted string actually returned by each DB, not
                            time.Time; settings JSON that (de)serializes itself)
    error/                 a portable sentinel for "unique constraint violated", since every
                            driver reports that differently
  adapter/
    http/                  the driving adapter: net/http handlers, routing, cookies, JSON
                            encoding/decoding, and use-case-error → HTTP-status mapping
    sqlite/, mysql/, postgres/
                            one driven adapter per supported database: connection setup,
                            idempotent schema migration, and Storage — all dialect-specific SQL
                            lives here (parameter placeholders, LAST_INSERT_ID vs RETURNING,
                            JSON_SET vs jsonb_set, …)
    filesystem/             stores/serves uploaded photo files from local disk

web/
  index.html, styles.css   the UI shell and styling
  app.js                   a single Vue 3 app (loaded from a CDN, no build step) — state,
                            computed properties, and every user action
  api.js                   a thin fetch() wrapper around the REST API
  i18n.js                  translation tables for the UI and for backend error messages
  files/                   runtime directory for uploaded photos (NOT embedded into the binary —
                            served straight off disk; see internal/adapter/filesystem)

build/docker/Dockerfile     minimal Alpine image that copies in a pre-built static binary
docker-compose.yml          a local MariaDB container for development
Makefile                    run / build / test / lint / docker-* targets
```

### Domain model

| Entity   | Fields |
|----------|--------|
| `User`   | id, username (unique, ≥3 chars), password (bcrypt hash, ≥4 chars), settings (`language`, `locationFilterDepth`) |
| `Session`| token (session cookie value), user id, expiry (30 days from login) |
| `Location` | id, name, color, optional parent id (nesting is unbounded) |
| `Item`   | id, name, location id, notes, photos (ordered list of file URLs), updatedAt |

A location can't be deleted while it still has nested locations or items in it — the API returns a
409 Conflict, and the frontend also pre-checks this to gray out the delete button. A location's
parent can't be changed once created; renaming/recoloring is supported, re-parenting isn't (by
design, not an oversight — see the doc comment on `location/update`).

## Getting started

### Requirements

- Go 1.26+ to build from source (a prebuilt binary needs nothing but a database).
- One of:
  - MySQL or MariaDB, with a user that can create the database (or a database created ahead of
    time);
  - PostgreSQL, likewise;
  - or nothing at all — SQLite just needs a writable path for its database file.

### Configuration

Copy `.env.example` to `.env` and adjust it — both `docker-compose` (for the local MariaDB
container) and the app itself (via [godotenv](https://github.com/joho/godotenv)) read it
automatically. Real environment variables always take priority over `.env`.

| Variable | Required | Default | Notes |
|---|---|---|---|
| `DB_DRIVER` | yes | — | `mysql`, `postgres` or `sqlite` |
| `DB_HOST` | mysql/postgres only | — | |
| `DB_PORT` | mysql/postgres only | — | typically `3306` / `5432` |
| `DB_USER` | mysql/postgres only | — | |
| `DB_PASSWORD` | no | empty | read raw, not trimmed — a password may legitimately contain spaces |
| `DB_NAME` | mysql/postgres only | — | created automatically if the user has the privilege |
| `DB_SQLITE_PATH` | sqlite only | — | database file, created (with parent dirs) on first run |
| `DB_MAX_OPEN_CONNS` | no | `10` | ignored for sqlite (always 1 — no concurrent writers) |
| `DB_MAX_IDLE_CONNS` | no | `5` | ignored for sqlite |
| `DB_CONN_MAX_LIFETIME` | no | `5m` | Go duration syntax |
| `DB_CONN_TIMEOUT` | no | `5s` | initial TCP dial timeout, mysql only |
| `PORT` | no | `8080` | HTTP port the server listens on |
| `COOKIE_SECURE` | no | `false` | set `true` in production (HTTPS) so session cookies require TLS |
| `LOG_CONSOLE_LEVEL` | no | `warn` | `debug` / `info` / `warn` / `error`, plain text to stdout |
| `LOG_FILE_PATH` | no | unset | JSON logs to a file, for a log aggregator; off unless set |
| `LOG_FILE_LEVEL` | no | `info` | only used if `LOG_FILE_PATH` is set |
| `DEMO_USERNAME` / `DEMO_PASSWORD` | no | `user` / `pass` | the demo account seeded on first run against an empty database |

A value that's set but malformed (`PORT=http`, `COOKIE_SECURE=yes`, …) fails startup immediately
with a specific error, rather than silently falling back to a default.

### Run it

```bash
# 1. Fetch dependencies
go mod tidy

# 2a. Easiest: SQLite, no server needed
export DB_DRIVER=sqlite
export DB_SQLITE_PATH=./data/whereabouts.db

# 2b. Or spin up a local MariaDB via docker-compose (reads .env)
docker-compose up -d
export DB_DRIVER=mysql   # plus DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME from .env

# 3. Run
go run ./cmd/whereabouts
```

On first run against an empty database the app creates the schema (idempotent `CREATE TABLE IF
NOT EXISTS` migrations — see `internal/adapter/<driver>/migrate.go`) and seeds a demo user. Open
`http://localhost:8080` and log in with `DEMO_USERNAME` / `DEMO_PASSWORD` (`user` / `pass` by
default).

If the configured database user can't create databases, create it by hand first:

```sql
-- MySQL/MariaDB
CREATE DATABASE whereabouts CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
-- PostgreSQL
CREATE DATABASE whereabouts;
```

### Creating additional accounts

There is no public registration screen — the API has no `/register` route. Accounts are created
with the binary's own subcommand, which loads the same DB config and exits without starting the
server:

```bash
go run ./cmd/whereabouts create-user -username=alice -password=hunter22
# or, once built:
./whereabouts create-user -username=alice -password=hunter22
```

### Building & running the binary

```bash
go build -o whereabouts ./cmd/whereabouts
./whereabouts
```

The binary already contains the entire frontend (`web/index.html`, `styles.css`, `app.js`,
`api.js`, `i18n.js`, icons) via `go:embed` — nothing else needs to be shipped alongside it except
the directory it's allowed to write uploaded photos into (`web/files/` by default, created
automatically).

`make build` cross-compiles a static (`CGO_ENABLED=0`) Linux/amd64 binary into `build/app/`, ready
to be copied into the Alpine-based image at `build/docker/Dockerfile`.

### Useful `make` targets

| Target | Does |
|---|---|
| `make run` | `go run ./cmd/whereabouts` |
| `make build` | cross-compile a static binary into `build/app/whereabouts` |
| `make test` | `go test ./...` |
| `make fmt` / `make vet` | `go fmt` / `go vet` |
| `make lint` | run `golangci-lint` (fetched into `./bin` by `make install-deps`) |
| `make docker-up` / `docker-down` / `docker-restart` / `docker-logs` / `docker-ps` | manage the local MariaDB container from `docker-compose.yml` |

## HTTP API

All responses are JSON; errors are `{"error": "<message>"}` with an appropriate HTTP status code
(400/401/403/404/409/500). Every `/api/user`, `/api/items*` and `/api/locations*` route requires a
valid session cookie and returns `401` without one.

| Method | Path | Description |
|---|---|---|
| GET | `/api/health` | liveness check |
| POST | `/api/auth/login` | `{username, password, language}` → sets the session cookie, returns the user |
| POST | `/api/auth/logout` | clears the session |
| GET | `/api/auth/me` | current user from the session cookie (used to restore a session after a page reload) |
| PATCH | `/api/user/language` | `{language}` — one of `en`/`ru`/`de`/`es`/`fr` |
| PATCH | `/api/user/location-filter-depth` | `{locationFilterDepth}` — `0` (all levels) or ≥ 1 |
| PATCH | `/api/user/username` | `{username, currentPassword}` |
| PATCH | `/api/user/password` | `{currentPassword, newPassword}` |
| GET | `/api/items` | list every item |
| POST | `/api/items` | `{name, locationId, notes, images}` — `images` is `data:` URLs and/or previously-returned `/files/...` URLs |
| PUT | `/api/items/{id}` | same body, replaces the item |
| DELETE | `/api/items/{id}` | delete an item |
| GET | `/api/locations` | flat list of every location, with `parentId` |
| POST | `/api/locations` | `{name, color, parentId}` |
| PUT | `/api/locations/{id}` | `{name, color}` — renaming/recoloring only, parent is fixed at creation |
| DELETE | `/api/locations/{id}` | `409` if it still has nested locations or items in it |
| GET | `/files/{name}` | serves a previously uploaded photo |

## Photo handling

The browser already downscales a picked photo to at most 1000px on its longest side before upload
(pure UX/bandwidth optimization — see `resizeImage` in `web/app.js`), but the server never trusts
that: every incoming photo goes through `internal/domain/service/imageprocessor`, dependency-free
and built entirely on the Go standard library's `image` package:

- A photo that's already a compact JPEG (≤ 350 KB) is left untouched.
- Otherwise it's decoded (JPEG/PNG/GIF), downscaled (bilinear interpolation) so neither side
  exceeds 1600px, and re-encoded as JPEG, stepping quality down from 85 to a floor of 40 until it
  lands under a ~700 KB budget.
- A format the standard library can't decode (HEIC, WebP, …) isn't rejected — it's stored as-is
  rather than losing the photo.
- Saved files get a random 16-byte hex name and live under `web/files/`, served back at
  `/files/<name>`; a request that touches photos is capped at 20 MB total.

## Internationalization

The UI supports English, Russian, German, Spanish and French. English strings are written directly
at each call site (`t('Add item')`) and used as-is; the other four languages are exact-match
translation tables in `web/i18n.js`.

The backend itself always answers in English — a full server-side i18n layer isn't worth it for
about fifty distinct messages — so the frontend translates known backend error strings by exact
match too (`SERVER_ERRORS` in `web/i18n.js`), with a handful of regexes for the few compound
messages that carry a dynamic suffix (a byte-size limit, a photo index, a wrapped inner error).
Anything the backend returns that isn't recognized is shown in English rather than silently
swallowed. A user's language choice is stored server-side (`users.settings.language`) so it follows
them across devices; the browser's own language and a local cache are only used before login, or
while that value hasn't loaded yet.

## Testing

The Go codebase has extensive unit test coverage: every use case, every repository, every database
adapter (against `sqlmock` for MySQL/Postgres and a real in-memory-ish SQLite file for that
driver), and the configuration loaders. Mocks for the `internal/port` interfaces are generated with
`go.uber.org/mock`; `stretchr/testify` provides assertions and `brianvoe/gofakeit` generates
realistic test data. Run everything with:

```bash
go test ./...
# or
make test
```
