# AGENTS.md

## What this is

EasyImage — a Go rewrite of a PHP image hosting service (图床). Single-binary Gin web server, HTML templates, file-based storage (no database). Stores images under `i/` with date-based subdirectories.

## Build and run

```bash
go build -o easyimage          # build
./easyimage                    # runs on :8080 (configurable in config/config.json)
docker-compose up -d           # Docker build + run
```

No tests, no linter, no formatter config exist in this repo. `go vet ./...` is the only available static check.

Docker build uses `CGO_ENABLED=0` — keep it that way.

## Architecture

- `main.go` — entrypoint, route registration, template function registration, auto-migration logic
- `config/` — config loading (`config.go`), PHP migration (`php_migrate.go`), PHP config files kept for migration
- `internal/handler/` — all HTTP handlers in a single `handler.go` file
- `internal/middleware/` — auth middleware (`CheckLogin`, `RequireAdmin`)
- `internal/service/` — business logic: `service.go` (auth, file ops, crypto), `image.go` (upload processing, watermark, compress), `captcha.go` (builtin/turnstile/recaptcha), `legacy_password.go` (SHA256 compat)
- `templates/` — 11 Go HTML templates loaded via `r.LoadHTMLGlob("templates/*")`
- `public/` — static assets served at `/public`
- `i/` — image storage, served at `/i` (route registered as `strings.TrimRight(cfg.Path, "/")`)
- `cmd/php2json/` — standalone tool to convert PHP config to JSON
- `cmd/migrate_test/` — migration test utility

## Key conventions

- Config is JSON (`config/config.json`), gitignored. PHP config files (`config.php`, `config.guest.php`, `api_key.php`) are version-controlled for migration.
- Config singleton: use `config.Get()` to read, `config.Save(cfg)` to write. Config is loaded once at startup.
- Handlers receive `*config.Config` as a closure parameter — do not use globals.
- Custom template functions are registered in `main.go` (`format_size`, `mul`, `div`, `minus`, `len`, `index`, `trimSuffix`, `now`). If you add a new template function, register it there.
- Version is in `config/config.go` (`var Version`). It is a `var` (not `const`) so Docker builds can override it via `-ldflags -X`. The release workflow auto-bumps it.
- Image paths in config use URL format (`/i/`), filesystem paths require `./i/` prefix. The handlers convert with `"." + cfg.Path`.
- Password hashing: new passwords use bcrypt (`service.HashPassword`), legacy PHP passwords use SHA256 (`legacy_password.go`). Both are checked in `ValidateLogin`.
- Auth is cookie-based (`auth` cookie with JSON-encoded `[user, password]`).

## Custom template `index` function

The repo overrides Go's built-in `index` with a variadic version in `main.go` that supports:
- Slice indexing: `index .dailyStats 5` (int key on `[]gin.H` or `[]interface{}`)
- Map key lookup: `index $last "Count"` (string key on `gin.H` or `map[string]interface{}`)
- Chained access: `index .dailyStats 5 "Count"` (slice then map in one call)

Without this, `{{index $map "key"}}` would fail because the custom function only accepted `int` keys.

## Go template pipe semantics (critical gotcha)

`x | f args` passes `x` as the **last** argument: `len .list | minus 1` = `minus(1, 30)` = `-29`, **not** `29`.

To compute `len - 1`, use explicit call syntax: `minus (len .list) 1`. Never pipe into `minus` when the piped value should be the first operand.

## WebP conversion

- Controlled by `webp_convert` (0/1) and `webp_quality` (default 80) in config
- WebP files stored in `i/webp/` mirroring original directory structure (e.g., `./i/webp/2026/05/08/xxx.webp`)
- WebP URLs returned in upload response as `webp_url` field
- Skips already-webp files and animated GIFs
- WebP files are served by the existing `/i` static route (e.g., `/i/webp/2026/05/08/xxx.webp`)

## Captcha

- Three types: builtin (math question), Cloudflare Turnstile, Google reCAPTCHA v3
- Controlled by `captcha` (0/1) and `captcha_type` (0/1/2) in config
- Builtin captcha tokens are HMAC-signed, expire in 5 minutes
- External captcha scripts (`turnstile/v0/api.js`, `recaptcha/api.js`) are preloaded via `<link rel="preload">` in `<head>` when captcha is enabled
- On the index page, captcha widgets are lazily initialized when the login modal opens (not on page load)
- When `mustLogin=1`, builtin captcha data is pre-fetched in the background on page load

## Release workflow

- `Version` in `config/config.go` is the single source of truth
- `.github/workflows/release.yml` — manually triggered, bumps version in `config/config.go`, commits, tags `vX.Y.Z`, creates GitHub Release
- Tag push triggers `.github/workflows/docker-image.yml` which builds Docker image with `VERSION` build arg passed via `-ldflags -X`
- Docker build: `--build-arg VERSION=X.Y.Z` overrides the default in the Dockerfile

## Admin routes

- `/admin/index` — login page
- `/admin/manager` — config management
- `/admin/chart` — statistics
- `/admin/history` — upload history
- `/admin/urllist` — image URL list with pagination and WebP URLs
- `/admin/filer` — file management
- `/api/urllist` — JSON API for image URL list

## Gotchas

- Template files must be UTF-8 without BOM. Corrupted encoding causes blank pages (silent template parse failure).
- `cfg.Path` is `/i/` (URL path). When calling `os.Stat`, `filepath.Walk`, or `filepath.Glob`, prepend `.` to get filesystem path (`./i/`).
- The `i/` directory and `config/config.json` are gitignored — they exist at runtime but not in the repo.
- Docker image deletes `config.json` during build to allow auto-migration detection on first run.
- No `go.sum` regeneration needed unless `go.mod` changes — run `go mod tidy` if you modify dependencies.
- Templates must use `{{.mustLogin}}` (passed from handler) to check login-only mode; `{{.config.MustLogin}}` also works since config is passed directly.

## File count

~9 Go source files, 11 HTML templates, no tests.
