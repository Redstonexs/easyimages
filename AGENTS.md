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
- `internal/service/` — business logic: `service.go` (auth, file ops, crypto), `image.go` (upload processing, watermark, compress), `legacy_password.go` (SHA256 compat)
- `templates/` — Go HTML templates loaded via `r.LoadHTMLGlob("templates/*")`
- `public/` — static assets served at `/public`
- `i/` — image storage, served at `/i` (route registered as `strings.TrimRight(cfg.Path, "/")`)
- `cmd/php2json/` — standalone tool to convert PHP config to JSON
- `cmd/migrate_test/` — migration test utility

## Key conventions

- Config is JSON (`config/config.json`), gitignored. PHP config files (`config.php`, `config.guest.php`, `api_key.php`) are version-controlled for migration.
- Config singleton: use `config.Get()` to read, `config.Save(cfg)` to write. Config is loaded once at startup.
- Handlers receive `*config.Config` as a closure parameter — do not use globals.
- Custom template functions are registered in `main.go` (`format_size`, `mul`, `div`, `minus`, `len`, `index`, `trimSuffix`, `now`). If you add a new template function, register it there.
- Version constant is in `config/config.go` (`const Version`).
- Image paths in config use URL format (`/i/`), filesystem paths require `./i/` prefix. The handlers convert with `"." + cfg.Path`.
- Password hashing: new passwords use bcrypt (`service.HashPassword`), legacy PHP passwords use SHA256 (`legacy_password.go`). Both are checked in `ValidateLogin`.
- Auth is cookie-based (`auth` cookie with JSON-encoded `[user, password]`).

## Gotchas

- Template files must be UTF-8 without BOM. Corrupted encoding causes blank pages (silent template parse failure).
- `cfg.Path` is `/i/` (URL path). When calling `os.Stat`, `filepath.Walk`, or `filepath.Glob`, prepend `.` to get filesystem path (`./i/`).
- The `i/` directory and `config/config.json` are gitignored — they exist at runtime but not in the repo.
- Docker image deletes `config.json` during build to allow auto-migration detection on first run.
- No `go.sum` regeneration needed unless `go.mod` changes — run `go mod tidy` if you modify dependencies.

## File count

~5 Go source files, 10 HTML templates, no tests.
