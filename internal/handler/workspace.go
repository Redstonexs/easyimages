package handler

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"easyimage/config"
	"easyimage/internal/middleware"
	"easyimage/internal/service"
	"easyimage/internal/workspace"

	"github.com/gin-gonic/gin"
	"go.yaml.in/yaml/v3"
)

//go:embed workspace.openapi.yaml
var workspaceAPISchema []byte

type workspaceHTTP struct {
	cfg            *config.Config
	store          *workspace.Store
	base           string
	transformCache *transformCache
}

type workspaceEndpoint func(*gin.Context, workspace.Identity) error

func RegisterWorkspaceRoutes(r *gin.Engine, cfg *config.Config, store *workspace.Store) {
	h := workspaceHTTP{cfg: cfg, store: store, base: workspaceAPIBase(cfg.Path), transformCache: newTransformCache()}
	r.GET("/workspace", h.workspacePage)
	r.GET("/admin/workspace", middleware.RequireAdmin(cfg), h.workspacePage)
	r.POST("/logout", h.logout)

	api := r.Group(h.base)
	schema := workspaceAPISchema
	if h.base != defaultWorkspaceAPIBase {
		schema = bytes.ReplaceAll(schema, []byte(defaultWorkspaceAPIBase+"/"), []byte(h.base+"/"))
	}
	schemaJSON := sync.OnceValues(func() ([]byte, error) {
		var spec any
		if err := yaml.Unmarshal(schema, &spec); err != nil {
			return nil, err
		}
		return json.Marshal(spec)
	})
	api.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml", schema)
	})
	api.GET("/openapi.json", func(c *gin.Context) {
		data, err := schemaJSON()
		if err != nil {
			writeWorkspaceError(c, err)
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})
	api.GET("/me", h.authenticate(h.me))
	api.GET("/users", h.authenticate(h.users))
	api.POST("/users", h.authenticate(h.createUser))
	api.PATCH("/users/:userID", h.authenticate(h.updateUser))
	api.POST("/tokens", h.authenticate(h.rotateToken))
	api.DELETE("/tokens", h.authenticate(h.revokeToken))
	api.GET("/folders", h.authenticate(h.folders))
	api.POST("/folders", h.authenticate(h.createFolder))
	api.GET("/images", h.authenticate(h.images))
	api.POST("/images", h.authenticate(h.uploadImage))
	api.GET("/images/:imageID", h.authenticate(h.image))
	api.DELETE("/images/:imageID", h.authenticate(h.deleteImage))
	api.POST("/images/:imageID/variants", h.authenticate(h.createVariant))
	api.DELETE("/images/:imageID/variants/:variantID", h.authenticate(h.deleteImage))
	api.GET("/settings", h.authenticate(h.getSettings))
	api.PUT("/settings", h.authenticate(h.updateSettings))

	r.NoRoute(h.matchDelivery, middleware.HotlinkProtection(cfg), h.deliverImage)
}

const defaultWorkspaceAPIBase = "/api/v1"

func workspaceAPIBase(legacyPath string) string {
	legacy := path.Clean(legacyPath) + "/"
	api := defaultWorkspaceAPIBase + "/"
	// Gin's wildcard for a custom legacy storage directory can already own
	// this namespace. Give its existing URLs priority over new routes.
	if strings.HasPrefix(legacy, api) || strings.HasPrefix(api, legacy) {
		return "/api/workspace/v1"
	}
	return defaultWorkspaceAPIBase
}

func (h workspaceHTTP) actor(c *gin.Context) (workspace.Identity, bool) {
	if auth := c.GetHeader("Authorization"); auth != "" {
		fields := strings.Fields(auth)
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			return workspace.Identity{}, false
		}
		return h.store.AuthenticateToken(fields[1])
	}
	session, ok := service.CurrentSession(c)
	if !ok {
		return workspace.Identity{}, false
	}
	if session.IsAdmin {
		return workspace.Identity{ID: workspace.AdminID}, true
	}
	actor := workspace.Identity{ID: session.UserID, AuthVersion: session.AuthVersion}
	return actor, h.store.Authorized(actor)
}

func (h workspaceHTTP) authenticate(endpoint workspaceEndpoint) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if h.store == nil {
			writeWorkspaceProblem(c, http.StatusServiceUnavailable, "workspace_unavailable", "The workspace catalog is unavailable.")
			return
		}
		actor, ok := h.actor(c)
		if !ok {
			c.Header("WWW-Authenticate", `Bearer realm="EasyImage"`)
			writeWorkspaceError(c, workspace.ErrUnauthorized)
			return
		}
		if c.GetHeader("Authorization") == "" && c.Request.Method != http.MethodGet && !h.sameOrigin(c) {
			writeWorkspaceError(c, workspace.ErrForbidden)
			return
		}
		if err := endpoint(c, actor); err != nil {
			writeWorkspaceError(c, err)
		}
	}
}

func (h workspaceHTTP) sameOrigin(c *gin.Context) bool {
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	raw := c.GetHeader("Origin")
	if raw == "" {
		return true
	}
	origin, err := url.Parse(raw)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil {
		return false
	}
	if strings.EqualFold(origin.Host, c.Request.Host) {
		return true
	}
	site, err := url.Parse(h.cfg.Domain)
	return err == nil && origin.Scheme == site.Scheme && strings.EqualFold(origin.Host, site.Host)
}

func (h workspaceHTTP) workspacePage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.store == nil {
		c.HTML(http.StatusServiceUnavailable, "error.html", gin.H{"config": h.cfg, "message": "图片库暂时不可用"})
		return
	}
	session, loggedIn := service.CurrentSession(c)
	if !loggedIn || (!session.IsAdmin && !h.store.Authorized(workspace.Identity{ID: session.UserID, AuthVersion: session.AuthVersion})) {
		if loggedIn {
			service.ClearSession(c, h.cfg)
		}
		c.Redirect(http.StatusFound, loginRedirectURL(c))
		return
	}
	if session.IsAdmin && c.Request.URL.Path == "/workspace" {
		c.Redirect(http.StatusFound, "/admin/workspace")
		return
	}
	c.HTML(http.StatusOK, "admin_manager.html", adminShellData(c, h.cfg, "workspace"))
}

func (h workspaceHTTP) logout(c *gin.Context) {
	if !h.sameOrigin(c) {
		c.Status(http.StatusForbidden)
		return
	}
	service.ClearSession(c, h.cfg)
	c.Redirect(http.StatusSeeOther, "/")
}

func (h workspaceHTTP) me(c *gin.Context, actor workspace.Identity) error {
	user, err := h.store.Me(actor, h.cfg.User)
	return workspaceJSON(c, http.StatusOK, user, err)
}

func (h workspaceHTTP) users(c *gin.Context, actor workspace.Identity) error {
	items, err := h.store.Users(actor)
	return workspaceJSON(c, http.StatusOK, gin.H{"items": items}, err)
}

func (h workspaceHTTP) createUser(c *gin.Context, actor workspace.Identity) error {
	if !actor.IsAdmin() {
		return workspace.ErrForbidden
	}
	var input workspace.CreateUser
	if err := readWorkspaceJSON(c, &input); err != nil {
		return err
	}
	user, err := h.store.CreateUser(actor, h.cfg.User, input)
	return workspaceJSON(c, http.StatusCreated, user, err)
}

func (h workspaceHTTP) updateUser(c *gin.Context, actor workspace.Identity) error {
	if !actor.IsAdmin() {
		return workspace.ErrForbidden
	}
	var input workspace.UpdateUser
	if err := readWorkspaceJSON(c, &input); err != nil {
		return err
	}
	user, err := h.store.UpdateUser(actor, c.Param("userID"), input)
	return workspaceJSON(c, http.StatusOK, user, err)
}

func (h workspaceHTTP) rotateToken(c *gin.Context, actor workspace.Identity) error {
	if c.GetHeader("Authorization") != "" {
		return workspace.ErrForbidden
	}
	token, err := h.store.RotateToken(actor)
	return workspaceJSON(c, http.StatusCreated, gin.H{"token": token, "token_type": "Bearer"}, err)
}

func (h workspaceHTTP) revokeToken(c *gin.Context, actor workspace.Identity) error {
	if c.GetHeader("Authorization") != "" {
		return workspace.ErrForbidden
	}
	if err := h.store.RevokeToken(actor); err != nil {
		return err
	}
	c.Status(http.StatusNoContent)
	return nil
}

func (h workspaceHTTP) folders(c *gin.Context, actor workspace.Identity) error {
	owner := c.Query("owner_id")
	withLegacy, err := includeLegacy(c, actor, owner)
	if err != nil {
		return err
	}
	items, err := h.store.Folders(actor, owner)
	if err != nil || !withLegacy {
		return workspaceJSON(c, http.StatusOK, gin.H{"items": items}, err)
	}
	folders := readLegacyFolders(h.cfg)
	for _, folder := range items {
		folders[folder.Path] = workspaceFolder{Folder: folder}
	}
	out := make([]workspaceFolder, 0, len(folders))
	for _, folder := range folders {
		out = append(out, folder)
	}
	slices.SortFunc(out, func(a, b workspaceFolder) int { return strings.Compare(a.Path, b.Path) })
	return workspaceJSON(c, http.StatusOK, gin.H{"items": out}, nil)
}

func (h workspaceHTTP) createFolder(c *gin.Context, actor workspace.Identity) error {
	var input struct {
		Path string `json:"path"`
	}
	if err := readWorkspaceJSON(c, &input); err != nil {
		return err
	}
	folder, err := h.store.CreateFolder(actor, c.Query("owner_id"), input.Path)
	return workspaceJSON(c, http.StatusCreated, folder, err)
}

type deliverySettings struct {
	workspace.Settings
	EffectiveOrigin string `json:"effective_origin"`
}

func (h workspaceHTTP) deliverySettings() deliverySettings {
	settings := h.store.Settings()
	origin := settings.PublicOrigin
	if origin == "" {
		origin = strings.TrimRight(h.cfg.Domain, "/")
	}
	return deliverySettings{Settings: settings, EffectiveOrigin: origin}
}

func (h workspaceHTTP) getSettings(c *gin.Context, actor workspace.Identity) error {
	if !actor.IsAdmin() {
		return workspace.ErrForbidden
	}
	return workspaceJSON(c, http.StatusOK, h.deliverySettings(), nil)
}

func (h workspaceHTTP) updateSettings(c *gin.Context, actor workspace.Identity) error {
	if !actor.IsAdmin() {
		return workspace.ErrForbidden
	}
	var input workspace.Settings
	if err := readWorkspaceJSON(c, &input); err != nil {
		return err
	}
	if _, err := h.store.UpdateSettings(actor, input); err != nil {
		return err
	}
	return workspaceJSON(c, http.StatusOK, h.deliverySettings(), nil)
}

type workspaceAsset struct {
	workspace.Asset
	URL     string `json:"url"`
	WebPURL string `json:"webp_url,omitempty"`
	Legacy  bool   `json:"legacy,omitempty"`
}

func (settings deliverySettings) asset(item workspace.Asset) workspaceAsset {
	out := workspaceAsset{Asset: item, URL: settings.EffectiveOrigin + settings.PublicPrefix + "/" + item.ID}
	if item.ContentType == "image/webp" {
		out.WebPURL = out.URL
	} else if query := workspace.WebPConversionQuery(item); query != "" {
		out.WebPURL = out.URL + query
	}
	return out
}

func (settings deliverySettings) assets(items []workspace.Asset) []workspaceAsset {
	out := make([]workspaceAsset, 0, len(items))
	for _, item := range items {
		out = append(out, settings.asset(item))
	}
	return out
}

func (h workspaceHTTP) images(c *gin.Context, actor workspace.Identity) error {
	filter := workspace.ImageFilter{OwnerID: c.Query("owner_id"), Cursor: c.Query("cursor"), Limit: 50}
	if raw, ok := c.GetQuery("limit"); ok {
		var err error
		filter.Limit, err = strconv.Atoi(raw)
		if err != nil {
			return workspace.ValidationError("limit must be an integer")
		}
	}
	if path, ok := c.GetQuery("folder"); ok {
		filter.Folder = &path
	}
	var err error
	filter.IncludeVariants, err = workspaceQueryBool(c, "include_variants")
	if err != nil {
		return err
	}
	withLegacy, err := includeLegacy(c, actor, filter.OwnerID)
	if err != nil {
		return err
	}
	var legacy map[string]workspaceAsset
	if withLegacy && filter.Limit >= 1 && filter.Limit <= 100 {
		legacy = readLegacyImages(h.cfg, filter.Folder)
		filter.Legacy = make([]workspace.Asset, 0, len(legacy))
		for _, item := range legacy {
			filter.Legacy = append(filter.Legacy, item.Asset)
		}
	}
	items, cursor, err := h.store.Images(actor, filter)
	if err != nil {
		return err
	}
	settings := h.deliverySettings()
	out := make([]workspaceAsset, 0, len(items))
	for _, item := range items {
		if old, ok := legacy[item.ID]; ok {
			out = append(out, old)
		} else {
			out = append(out, settings.asset(item))
		}
	}
	return workspaceJSON(c, http.StatusOK, gin.H{"items": out, "next_cursor": cursor}, nil)
}

func workspaceQueryBool(c *gin.Context, name string) (bool, error) {
	if raw, ok := c.GetQuery(name); ok {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return false, workspace.ValidationError(name + " must be true or false")
		}
		return enabled, nil
	}
	return false, nil
}

func (h workspaceHTTP) image(c *gin.Context, actor workspace.Identity) error {
	id := c.Param("imageID")
	item, variants, err := h.store.Image(actor, id)
	if errors.Is(err, workspace.ErrNotFound) && actor.IsAdmin() && len(id) == 32 {
		if old, ok := findLegacyImageByID(h.cfg, id); ok {
			return workspaceJSON(c, http.StatusOK, gin.H{"image": old, "variants": []workspaceAsset{}}, nil)
		}
	}
	settings := h.deliverySettings()
	return workspaceJSON(c, http.StatusOK, gin.H{"image": settings.asset(item), "variants": settings.assets(variants)}, err)
}

func (h workspaceHTTP) uploadImage(c *gin.Context, actor workspace.Identity) error {
	const multipartOverhead = 1 << 20
	maxSize := min(h.cfg.MaxSize, math.MaxInt64-multipartOverhead)
	bodyLimit := maxSize + multipartOverhead
	if c.Request.ContentLength > bodyLimit {
		return workspace.ErrTooLarge
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, bodyLimit)
	if err := c.Request.ParseMultipartForm(multipartOverhead); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			return workspace.ErrTooLarge
		}
		return workspace.ValidationError("expected multipart/form-data with a file field")
	}
	form := c.Request.MultipartForm
	defer form.RemoveAll()
	if len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value["folder"]) > 1 {
		return workspace.ValidationError("provide one file and at most one folder field")
	}
	header := form.File["file"][0]
	if !service.IsAllowedExtension(header.Filename, h.cfg) {
		return workspace.ErrUnsupportedImage
	}
	file, err := header.Open()
	if err != nil {
		return err
	}
	defer file.Close()
	owner, folder := c.Query("owner_id"), c.PostForm("folder")
	item, err := h.store.Upload(actor, owner, folder, header.Filename, file, header.Size, maxSize)
	if errors.Is(err, workspace.ErrFolderNotFound) {
		bound, bindErr := h.useLegacyFolder(actor, owner, folder)
		if bindErr != nil {
			return bindErr
		}
		if bound {
			// The first upload attempt checks the destination before reading the file.
			item, err = h.store.Upload(actor, owner, folder, header.Filename, file, header.Size, maxSize)
		}
	}
	if err == nil {
		c.Header("Location", h.base+"/images/"+item.ID)
	}
	return workspaceJSON(c, http.StatusCreated, h.deliverySettings().asset(item), err)
}

func (h workspaceHTTP) createVariant(c *gin.Context, actor workspace.Identity) error {
	var input struct {
		Folder    *string                    `json:"folder"`
		Transform workspace.TransformOptions `json:"transform"`
	}
	if err := readWorkspaceJSON(c, &input); err != nil {
		return err
	}
	id := c.Param("imageID")
	item, err := h.store.CreateVariant(c.Request.Context(), actor, id, input.Folder, input.Transform)
	if errors.Is(err, workspace.ErrFolderNotFound) && actor.IsAdmin() && input.Folder != nil {
		original, _, lookupErr := h.store.Image(actor, id)
		if lookupErr != nil {
			return lookupErr
		}
		bound, bindErr := h.useLegacyFolder(actor, original.OwnerID, *input.Folder)
		if bindErr != nil {
			return bindErr
		}
		if bound {
			item, err = h.store.CreateVariant(c.Request.Context(), actor, id, input.Folder, input.Transform)
		}
	}
	response := h.deliverySettings().asset(item)
	if err == nil {
		c.Header("Location", response.URL)
	}
	return workspaceJSON(c, http.StatusCreated, response, err)
}

func (h workspaceHTTP) deleteImage(c *gin.Context, actor workspace.Identity) error {
	if err := h.store.Delete(actor, c.Param("imageID"), c.Param("variantID")); err != nil {
		return err
	}
	c.Status(http.StatusNoContent)
	return nil
}

func (h workspaceHTTP) matchDelivery(c *gin.Context) {
	if h.store == nil || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
		c.Abort()
		return
	}
	id := h.store.DeliveryID(c.Request.URL.Path)
	if id == "" {
		c.Abort()
		return
	}
	c.Set("deliveryImageID", id)
}

func (h workspaceHTTP) deliverImage(c *gin.Context) {
	id := c.GetString("deliveryImageID")
	asset, err := h.store.LookupPublic(id)
	if err != nil {
		writeWorkspaceError(c, err)
		return
	}

	var opts workspace.TransformOptions
	var transform bool
	if c.Request.URL.RawQuery != "" {
		query, err := url.ParseQuery(c.Request.URL.RawQuery)
		if err != nil {
			writeWorkspaceError(c, workspace.ValidationError("invalid image query parameters"))
			return
		}
		opts, transform, err = workspace.TransformFromQuery(query)
		if err != nil {
			writeWorkspaceError(c, err)
			return
		}
	}

	contentType := asset.ContentType
	etag := `"` + id + `"`
	cacheControl := c.Writer.Header().Get("Cache-Control")
	if cacheControl == "" {
		cacheControl = "public, max-age=300"
	}

	var body io.ReadSeeker
	if transform {
		opts, err = opts.Normalize(contentType)
		if err != nil {
			writeWorkspaceError(c, err)
			return
		}
		etag = transformETag(id, opts)
		if ifNoneMatch(c, etag) {
			c.Header("Cache-Control", cacheControl)
			c.Header("ETag", etag)
			c.Status(http.StatusNotModified)
			return
		}
		if data := h.transformCache.get(etag, time.Now()); data != nil {
			body, contentType = bytes.NewReader(data), "image/"+opts.Format
		} else {
			file, err := h.store.OpenAsset(asset)
			if err != nil {
				writeWorkspaceError(c, err)
				return
			}
			defer file.Close()
			rendered, err := h.store.Render(c.Request.Context(), file, contentType, opts)
			if err != nil {
				writeWorkspaceError(c, err)
				return
			}
			body, contentType = rendered.File, rendered.ContentType
			if rendered.SizeBytes <= maxCachedTransformSize {
				data := make([]byte, rendered.SizeBytes)
				if _, err := rendered.File.ReadAt(data, 0); err == nil {
					h.transformCache.put(etag, data, time.Now())
					body = bytes.NewReader(data)
					rendered.Remove()
					rendered = nil
				}
			}
			if rendered != nil {
				defer rendered.Remove()
			}
		}
	} else {
		if ifNoneMatch(c, etag) {
			c.Header("Cache-Control", cacheControl)
			c.Header("ETag", etag)
			c.Status(http.StatusNotModified)
			return
		}
		file, err := h.store.OpenAsset(asset)
		if err != nil {
			writeWorkspaceError(c, err)
			return
		}
		defer file.Close()
		body = file
	}

	c.Header("Cache-Control", cacheControl)
	c.Header("ETag", etag)
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	if contentType == "image/svg+xml" {
		c.Header("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	}
	http.ServeContent(c.Writer, c.Request, asset.Filename, asset.CreatedAt, body)
}

func transformETag(id string, opts workspace.TransformOptions) string {
	var buf [128]byte
	key := append(buf[:0], "v2/"...)
	key = append(key, id...)
	key = append(key, '/')
	key = strconv.AppendInt(key, int64(opts.Width), 10)
	key = append(key, 'x')
	key = strconv.AppendInt(key, int64(opts.Height), 10)
	key = append(key, '/')
	key = strconv.AppendFloat(key, opts.Scale, 'g', -1, 64)
	key = append(key, '/')
	key = append(key, opts.Fit...)
	key = append(key, '/')
	key = append(key, opts.Format...)
	key = append(key, '/')
	key = strconv.AppendInt(key, int64(opts.Quality), 10)
	hash := sha256.Sum256(key)
	var out [3 + 64 + 1]byte
	out[0], out[1], out[2] = 'W', '/', '"'
	const hexDigits = "0123456789abcdef"
	for i, b := range hash {
		out[3+i*2] = hexDigits[b>>4]
		out[3+i*2+1] = hexDigits[b&0x0f]
	}
	out[len(out)-1] = '"'
	return string(out[:])
}

func ifNoneMatch(c *gin.Context, etag string) bool {
	if c.GetHeader("If-Match") != "" || c.GetHeader("If-Unmodified-Since") != "" {
		return false
	}
	for _, value := range strings.Split(c.GetHeader("If-None-Match"), ",") {
		if strings.TrimPrefix(strings.TrimSpace(value), "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}

func readWorkspaceJSON(c *gin.Context, dst any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			return workspace.ErrTooLarge
		}
		return workspace.ValidationError("invalid JSON request: " + err.Error())
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return workspace.ValidationError("expected one JSON object")
	}
	return nil
}

func workspaceJSON(c *gin.Context, status int, data any, err error) error {
	if err == nil {
		c.JSON(status, data)
	}
	return err
}

type workspaceProblem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
	*workspace.QuotaError
}

func writeWorkspaceProblem(c *gin.Context, status int, code, detail string) {
	writeWorkspaceProblemDetails(c, workspaceProblem{Status: status, Code: code, Detail: detail})
}

func writeWorkspaceProblemDetails(c *gin.Context, problem workspaceProblem) {
	problem.Type, problem.Title = "about:blank", http.StatusText(problem.Status)
	data, _ := json.Marshal(problem)
	c.Header("Cache-Control", "no-store")
	c.Data(problem.Status, "application/problem+json", data)
}

func writeWorkspaceError(c *gin.Context, err error) {
	var validation workspace.ValidationError
	var quota *workspace.QuotaError
	status, code := http.StatusBadRequest, "invalid_request"
	switch {
	case errors.As(err, &validation):
	case errors.As(err, &quota):
		writeWorkspaceProblemDetails(c, workspaceProblem{Status: http.StatusConflict, Code: "quota_exceeded", Detail: quota.Error(), QuotaError: quota})
		return
	case errors.Is(err, workspace.ErrUnauthorized):
		status, code = http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, workspace.ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, workspace.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, workspace.ErrFolderNotFound):
		status, code = http.StatusNotFound, "folder_not_found"
	case errors.Is(err, workspace.ErrConflict):
		status, code = http.StatusConflict, "already_exists"
	case errors.Is(err, workspace.ErrTooLarge):
		status, code = http.StatusRequestEntityTooLarge, "payload_too_large"
	case errors.Is(err, workspace.ErrUnsupportedImage):
		status, code = http.StatusUnsupportedMediaType, "unsupported_image"
	case errors.Is(err, workspace.ErrCannotTransform):
		status, code = http.StatusUnprocessableEntity, "cannot_transform"
	case errors.Is(err, workspace.ErrWebPUnavailable):
		status, code = http.StatusServiceUnavailable, "webp_unavailable"
	case errors.Is(err, workspace.ErrRenderBusy):
		c.Header("Retry-After", "1")
		status, code = http.StatusServiceUnavailable, "transform_busy"
	default:
		log.Printf("Workspace request failed: %v", err)
		writeWorkspaceProblem(c, http.StatusInternalServerError, "internal_error", "The request could not be completed.")
		return
	}
	writeWorkspaceProblem(c, status, code, err.Error())
}
