# AGENTS.md

## What this is

EasyImage is a Go rewrite of a PHP image host (图床): one Gin binary, Go HTML templates, file-based config/storage, no database. Runtime images live under `i/`; runtime logs live under `admin/logs/`.

## Commands

- Build frontend assets from the repo root: `npm run build`.
- Build the server from the repo root: `go build -o easyimage .`.
- Run locally: `./easyimage` (or `./easyimage.exe` on Windows). It listens on `:8080` unless `config/config.json` sets `port`.
- Docker run/build: `docker-compose up -d`.
- Static check: `go vet ./...`. There is no repo linter, formatter config, task runner, or test suite; `go test ./...` is only a compile/no-test sanity check.
- Manual PHP config converter: `go run ./cmd/php2json config/config.php config/config.json`.
- Migration status utility: `go run ./cmd/migrate_test`.

## Runtime state

- `config/config.json`, `config/config.guest.json`, `config/api_key.json`, `config/install.lock`, `config/php_backup/`, `admin/logs/`, and `i/` are gitignored runtime state. Do not commit local values from them.
- PHP source config files in `config/*.php` are versioned for migration. Startup runs `checkAndMigrate()` before `config.Load()` and skips migration when `config/config.json` is an installed or non-default Go config.
- Dockerfile intentionally removes generated JSON config and install locks during image build so first container start can auto-detect PHP configs.

## Architecture notes

- `main.go` is the wiring file: auto-migration, config load, directory creation, Gin routes, template funcs, static file routes, and server timeouts.
- `config/config.go` owns the config singleton. Use `config.Load()` at startup, `config.Get()` only after load, and `config.Save(cfg)` for writes. Handlers receive `*config.Config` closures; do not introduce new global config reads in handlers.
- `internal/handler/handler.go` contains nearly all HTTP handlers. `internal/service/` owns auth, file operations, image processing, captcha, and legacy password compatibility.
- Real upload API route is `POST /api/index`; do not document legacy PHP-style `/api/index.php` unless a compatibility route is added.
- Image URLs use `cfg.Path` such as `/i/`; filesystem access needs the local prefix (`"." + cfg.Path`). Be careful with `filepath.Join(".", cfg.Path, ...)` because a leading slash in `cfg.Path` can discard the dot on Unix.
- Static image serving registers `strings.TrimRight(cfg.Path, "/")` and wraps it with `middleware.HotlinkProtection(cfg)`.

## Templates

- Templates are loaded with `r.LoadHTMLGlob("templates/*")`; any new template function must be registered in `main.go` before this call.
- The custom `index` template function in `main.go` supports chained slice/map access such as `{{index .dailyStats 5 "Count"}}`; do not replace it with a simple int-only helper.
- Go template pipes pass the piped value as the last argument: `len .list | minus 1` means `minus(1, len)`. Use `minus (len .list) 1` when computing `len - 1`.
- The index page receives both `.config` and `.mustLogin`; existing captcha/login template logic relies on `.mustLogin`.

## Auth and security

- Sessions are in-memory tokens in the `session` cookie (`service.sessionStore`, 14-day max age). Process restarts invalidate sessions.
- New passwords are bcrypt (`service.HashPassword`); migrated PHP passwords can still validate as SHA256 through `legacy_password.go`.
- Admin settings updates in `ManagerAction` intentionally do not change sensitive fields like `Password`, `User`, `Path`, `Port`, and `HideKey`.
- Captcha modes are builtin math (`captcha_type=0`), Cloudflare Turnstile (`1`), and reCAPTCHA (`2`). Missing external captcha keys fall back to builtin; builtin tokens are HMAC-signed from the config password and expire after 5 minutes.

## Image processing

- WebP conversion is controlled by `webp_convert` and `webp_quality`; it requires the external `cwebp` CLI. Docker installs `libwebp-tools`, but local runs need `cwebp` on `PATH`.
- WebP files are stored under `i/webp/` mirroring the original directory tree and are returned as `webp_url` when present. Existing `.webp` files and animated GIFs are skipped.
- Batch WebP generation is `POST /admin/batch-webp` and requires admin auth plus `webp_convert=1`.

## Release workflow

- `config.Version` in `config/config.go` is a `var` so Docker builds can override it with `-ldflags -X easyimage/config.Version=...`.
- `.github/workflows/release.yml` manually bumps `config.Version`, commits `release: vX.Y.Z`, tags `vX.Y.Z`, and pushes to `master`.
- `.github/workflows/docker-image.yml` builds Docker images on `master`, tags, and manual dispatch; it passes `VERSION` as a Docker build arg. Keep Docker builds `CGO_ENABLED=0`.
