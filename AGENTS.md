# AGENTS.md

## What This Is

- EasyImage is a Go 1.21/Gin rewrite of a PHP image host: one binary, Go HTML templates, Vue/Vite islands, file-based config/storage, no database.
- Runtime images live under `i/`; runtime logs, image metadata, and S3 multipart state live under `admin/logs/`; custom site icons live under `config/site-icon/`; frontend build output is `public/dist/`.
- PHP files in `config/*.php` are versioned migration templates/input, not runtime config.

## Commands

- Install frontend deps: `npm ci`.
- Frontend typecheck: `npm run typecheck` (`vue-tsc --noEmit`).
- Frontend build: `npm run build` (`vite build`, writes gitignored `public/dist/`).
- All Go tests: `go test ./...`.
- Focused Go tests: `go test ./internal/service -run TestName`, `go test ./internal/handler -run TestName`, `go test ./internal/middleware -run TestName`, `go test ./internal/storage -run TestName`, or `go test . -run TestName` for root tests.
- Static check: `go vet ./...`.
- Build server: `go build -o easyimage .` or `go build -o easyimage.exe .` on Windows.
- Run locally after building assets: `./easyimage` or `./easyimage.exe`; default port is `:8080` unless `config/config.json` sets `port`.
- Docker Compose: `docker-compose up -d`; it builds assets in Node 22, builds the Go binary, and mounts `./config`, `./i`, and `./admin/logs`.
- Manual PHP config converter: `go run ./cmd/php2json config/config.php config/config.json`.
- Migration status utility: `go run ./cmd/migrate_test`.

## Generated And Runtime State

- Do not commit local runtime files: `config/config.json`, `config/config.guest.json`, `config/api_key.json`, `config/install.lock`, `config/EasyImage.lock`, `config/php_backup/`, `config/site-icon/`, `admin/logs/`, or files under `i/` except `i/.gitkeep`.
- Do not commit `public/dist/`; missing `public/dist/.vite/manifest.json` only logs a warning and skips Vue asset tags, but the Vue UI will not mount until `npm run build` runs.
- Dockerfile deletes generated JSON config and install locks during image build so first container start can auto-detect copied PHP configs.

## Architecture Map

- `main.go` is the wiring file: `checkAndMigrate()` before `config.Load()`, runtime directory creation, optional `cwebp` warning, chunk cleanup goroutine, Gin routes, template funcs, static routes, and 5-minute upload server timeouts.
- `config/config.go` owns cached singletons for main config, guest config, and API keys. `config.Load()` returns defaults without saving when JSON is absent; `config.Save(cfg)` writes `config/config.json`.
- `internal/handler/handler.go` contains HTML handlers, install, upload, chunk upload, API upload, and legacy admin actions.
- `internal/handler/admin_api.go` contains Vue admin JSON APIs under `/admin/api/*`, site icon upload/serve logic, and secret presence flags instead of external captcha or S3 secret values.
- `internal/handler/list_payload.go` builds shared gallery/history payloads for `/api/list`, `/app/list`, and admin history APIs.
- `internal/service/` owns auth/session, file listing/deletion, path safety helpers, image processing, captcha, metadata, and legacy password compatibility.
- `internal/middleware/` owns login/admin gates and hotlink protection.
- `templates/` host Vue islands; `web/src/` is Vue/TypeScript source; `public/static/` is legacy static asset content.

## Routes And Payloads

- Route source of truth is `main.go`; do not document PHP-style compatibility routes unless code adds them.
- Real API upload route is `POST /api/index`; there is no `/api/index.php` route.
- Web upload route is `POST /app/upload` and expects multipart field `file`; API upload expects field `image` plus `token` from `config/api_key.json`.
- Web upload accepts optional `storage_source`; omitted/disabled sources fall back through `cfg.StorageSourceByID` to the default source and then local.
- Admin HTML routes are under `/admin`; Vue admin data routes are under `/admin/api/*`.
- Public image list is `GET /api/list`; it validates `date`, `ext`, `q/search`, caps `num` at 500, and searches original filenames through metadata.

## Config And Migration

- Startup skips PHP migration when `config/config.json` is installed/customized or `config/install.lock` exists.
- Bundled default `config/*.php` templates do not trigger migration; copied real PHP configs do.
- Auto-migration backs PHP configs up to `config/php_backup/`, migrates PHP data dirs, writes `config/config.json`, `config/config.guest.json`, `config/api_key.json`, and creates `config/install.lock`.
- Default config values live in `config/config.go`; notable defaults are `Path=/i/`, `StoragePath=Y/m/d/`, `Port=8080`, `ListNumber=20`, `ListDate=10`, `SiteIcon=/favicon.ico`, `DefaultStorageSource=local`, and new installs `MustLogin=1`.
- Private upload mode is the existing `MustLogin` field and `middleware.CheckLogin(cfg)` on `/app/upload` and chunk routes; API upload stays token-authenticated and is not gated by `MustLogin`.
- Keep `MustLogin=1` in `getDefaultConfig()` for new installs only; do not set it in `setDefaults()` or old `mustLogin:0` configs would be silently changed.
- Handlers are wired with `*config.Config`; prefer passing that through instead of adding new global `config.Get()` reads in handlers.
- Tests often `os.Chdir(t.TempDir())`; config package globals can stay cached, so isolate config-touching tests carefully.

## Storage Sources

- Storage sources are configured by `Config.DefaultStorageSource` and `Config.StorageSources`; `ensureStorageDefaults` always preserves an enabled `local` source.
- Admin config edits storage sources as JSON. `s3_access_key_secret` is omitted on read; an empty secret on save preserves the existing secret for that source ID.
- S3-compatible sources use AWS SDK v2, support custom endpoint, region, bucket, prefix, path-style mode, and optional public base URL.
- Local uploads run SVG file scanning and async compression/watermark/WebP post-processing; S3 uploads go directly to object storage and store the public URL in metadata.
- Normal S3 SVG uploads are scanned in memory before `PutObject`; S3 multipart SVG uploads are intentionally rejected.
- S3 multipart state is file-backed under `admin/logs/multipart/`; completed S3 objects, deletes, details, thumbnails, and history depend on `ImageMetadata` fields (`storage_source`, `storage_type`, `object_key`, `url`, `thumb_url`).

## Frontend Notes

- Vite inputs are exactly `web/src/entries/upload.ts`, `web/src/entries/gallery.ts`, and `web/src/entries/admin.ts`; template `{{vite "..."}}` keys must match those manifest keys.
- Vite `base` is `/public/dist/` and `publicDir` is `false`; root `public/` is served by Gin, not copied by Vite.
- Vue mounts use `window.EasyImageUpload`, `window.EasyImageGallery`, and `window.EasyImageAdmin` bootstrap globals defined in templates after module script tags; this relies on module scripts being deferred.
- Admin templates all use `admin.ts`; the bootstrap only provides `{view, version, title}` and data is fetched from `/admin/api/*`.
- Service worker is manually registered at `/public/dist/sw.js` with scope `/`; `frontend_cache.go` sets `Service-Worker-Allowed: /` only for that path.
- Keep Go JSON response keys aligned with `web/src/types.ts`; TS is strict and only includes `web/**/*.ts` and `web/**/*.vue`.
- Admin file-browser payloads must return arrays, not `null`, for `dirs` and `files`; `AdminFiler.vue` uses these for `.length`, and `nil` slices previously caused blank pages at leaf directories.
- There is no npm `dev` script, ESLint, or Prettier config in this repo.

## Templates

- Template funcs must be registered in `main.go` before `r.LoadHTMLGlob("templates/*")`.
- The custom `index` template func supports chained slice/map access such as `{{index .dailyStats 5 "Count"}}`; do not replace it with a simple int-only helper.
- Go template pipes pass the piped value as the last argument: `len .list | minus 1` calls `minus(1, len)`, so use `minus (len .list) 1` for `len - 1`.
- The index page receives both `.config` and `.mustLogin`; captcha/login template logic relies on `.mustLogin`.
- Bootstrap JSON is emitted through `json_script` (`template.JS`); avoid hand-written JS object interpolation.

## Path And File Safety

- `cfg.Path` is a URL prefix like `/i/`; filesystem access normally needs `"." + cfg.Path` or an existing safety helper.
- Be careful with `filepath.Join(".", cfg.Path, ...)`: leading `/i/` can discard the dot on Unix.
- URL paths should go through `service.ValidateURLPath`; filesystem paths under storage should use `getSafePath*` or `SanitizePath`, which rebuild paths from a trusted storage root with `filepath.Rel`.
- Static image serving registers `strings.TrimRight(cfg.Path, "/")` and wraps it with `middleware.HotlinkProtection(cfg)`.
- Internal storage dirs are intentionally hidden from lists/file browsers: `cache`, `suspic`, `recycle`, `chunks`, `admin`, and `webp`.
- Image metadata is stored monthly as `admin/logs/metadata/YYYY-MM.json`; original upload names power display/search.
- `/favicon.ico` is dynamic: `handler.SiteIcon` serves `config/site-icon/favicon.{ico,png,svg}` first, then falls back to `public/images/image_icon_153794.png`; templates use `.config.SiteIcon` for cache busting.

## Auth And Security

- Sessions are in-memory `sync.Map` tokens in the `session` cookie with 14-day max age; process restarts invalidate sessions.
- Session cookie `Secure` is set only when `cfg.Domain` starts with `https`.
- Login rate limiting is per `ClientIP`: 5 failed attempts within 5 minutes.
- New passwords are bcrypt; migrated PHP passwords can still validate as SHA256 through `legacy_password.go`.
- `ManagerAction` intentionally does not update sensitive fields like `Password`, `User`, `Path`, `Port`, and `HideKey`.
- Captcha modes are builtin math (`captcha_type=0`), Cloudflare Turnstile (`1`), and reCAPTCHA (`2`). Missing external captcha keys fall back to builtin; builtin tokens are HMAC-signed from the config password and expire after 5 minutes.
- Hotlink protection allows empty `Referer`, the configured site host, or comma-separated whitelist domains/subdomains.

## Image Processing

- Local uploads call `service.ProcessUpload`/`ProcessLocalUpload`, which validates size/extension, stores under expanded `cfg.StoragePath`, queues post-processing, and records metadata.
- SVG uploads are saved first, then scanned by `CheckSVGSecurity`; unsafe SVGs are deleted.
- Post-processing is asynchronous behind a `runtime.NumCPU()*64` worker queue; if full, it falls back to a goroutine.
- Compression, watermarking, format conversion, and WebP conversion are configured independently.
- WebP conversion is controlled by `webp_convert` and `webp_quality`; it requires external `cwebp`. Docker installs `libwebp-tools`, local runs need `cwebp` on `PATH`.
- WebP files are stored under `i/webp/` mirroring the original tree; existing `.webp` files and animated GIFs are skipped.
- Batch WebP generation is `POST /admin/batch-webp` or `POST /admin/api/batch-webp`; it requires admin auth and `webp_convert=1`.

## Tests

- The repo has real Go tests; do not treat `go test ./...` as compile-only.
- `internal/service/image_test.go` fakes `cwebp` by re-executing the test binary with `FAKE_CWEBP=1`; avoid changing that `TestMain` flow casually.
- Many tests change cwd into `t.TempDir()` and restore it; avoid parallelizing those tests unless they are refactored for isolation.

## CI And Release

- CodeQL runs on `master` push/PR and weekly, analyzing Go with autobuild and JS/TS with no build.
- Release workflow is manual on `master`: bumps `config.Version`, commits `release: vX.Y.Z`, tags, pushes, and creates a GitHub release.
- `config.Version` is a `var` so Docker builds can override it with `-ldflags -X easyimage/config.Version=...`.
- Docker image workflow builds on pushes to `dev`, after a successful Release workflow from `master`, or manual dispatch on `dev`; release images are tagged `latest`, version, and `vversion`, while dev images use `dev` and `dev-sha`.
- Keep Docker builds `CGO_ENABLED=0`; the Dockerfile passes `VERSION` as a build arg and installs runtime `libwebp-tools` for `cwebp`.
