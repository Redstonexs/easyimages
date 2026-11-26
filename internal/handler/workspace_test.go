package handler

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"easyimage/config"
	"easyimage/internal/middleware"
	"easyimage/internal/service"
	"easyimage/internal/workspace"

	"github.com/gin-gonic/gin"
)

func TestWorkspaceIsolationAndPublicDelivery(t *testing.T) {
	cfg := workspaceTestConfig()
	runtimeDir := t.TempDir()
	store := openWorkspaceForTest(t, runtimeDir, cfg.Path)
	legacyDir := t.TempDir()
	original := workspaceTestPNG(t)
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy.png"), original, 0600); err != nil {
		t.Fatal(err)
	}
	router := workspaceTestRouter(cfg, store, legacyDir)
	admin := workspaceTestLogin(t, router, cfg.User, "legacy-password")
	studio := workspaceTestResponse[workspace.User](t, workspaceTestJSON(router, "POST", "/api/v1/users", admin, `{"username":"studio","password":"studio-password","quota_bytes":1048576}`), 201)
	workspaceTestStatus(t, workspaceTestJSON(router, "POST", "/api/v1/users", admin, `{"username":"observer","password":"observer-password"}`), 201)
	tenant := workspaceTestLogin(t, router, "studio", "studio-password")
	observer := workspaceTestLogin(t, router, "observer", "observer-password")
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", "/api/v1/users", tenant, ""), 403)
	workspaceTestStatus(t, workspaceTestJSON(router, "POST", "/app/upload", tenant, ""), 403)
	workspaceTestStatus(t, workspaceTestJSON(router, "POST", "/api/v1/folders", tenant, `{"path":"../legacy"}`), 400)
	workspaceTestStatus(t, workspaceTestJSON(router, "POST", "/api/v1/folders", tenant, `{"path":"projects/素材"}`), 201)
	workspaceTestStatus(t, workspaceTestJSON(router, "POST", "/api/v1/folders?owner_id="+studio.ID, admin, `{"path":"exports/small"}`), 201)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", "/api/v1/folders?owner_id="+studio.ID, observer, ""), 403)

	token := workspaceTestResponse[struct{ Token string }](t, workspaceTestJSON(router, "POST", "/api/v1/tokens", tenant, ""), 201).Token
	image := workspaceTestResponse[workspaceAsset](t, workspaceTestUpload(router, token, "projects/素材", original), 201)
	if image.OwnerID != studio.ID || image.Folder != "projects/素材" || image.Width != 96 || image.Height != 48 {
		t.Fatalf("unexpected image metadata: %+v", image)
	}
	href := workspaceTestPath(t, image.URL)
	if href != "/media/"+image.ID || strings.Contains(image.URL, "studio") {
		t.Fatalf("internal path leaked in public URL: %s", image.URL)
	}
	detail := "/api/v1/images/" + image.ID
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", detail, observer, ""), 404)
	workspaceTestStatus(t, workspaceTestJSON(router, "DELETE", detail, observer, ""), 404)
	workspaceTestStatus(t, workspaceTestJSON(router, "POST", detail+"/variants", observer, `{"transform":{"width":24}}`), 404)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", detail, admin, ""), 200)
	if got := workspaceTestJSON(router, "GET", href, nil, ""); got.Code != 200 || !bytes.Equal(got.Body.Bytes(), original) {
		t.Fatalf("original was changed: HTTP %d", got.Code)
	}
	if got := workspaceTestJSON(router, "HEAD", href, nil, ""); got.Code != 200 || got.Body.Len() != 0 || got.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("unexpected HEAD response: HTTP %d, %v", got.Code, got.Header())
	}
	dynamic := href + "?width=24&format=jpeg"
	got := workspaceTestJSON(router, "GET", dynamic, nil, "")
	checkWorkspaceImage(t, got, "image/jpeg", 24, 12)
	conditional := httptest.NewRequest("GET", dynamic, nil)
	conditional.Header.Set("If-None-Match", got.Header().Get("ETag"))
	workspaceTestStatus(t, runWorkspaceRequest(router, conditional), 304)
	checkWorkspaceImage(t, workspaceTestJSON(router, "GET", href+"?scale=0.5&format=png", nil, ""), "image/png", 48, 24)
	checkWorkspaceImage(t, workspaceTestJSON(router, "GET", href+"?width=17&height=17&fit=cover", nil, ""), "image/png", 17, 17)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", href+"?width=16&scale=0.5", nil, ""), 400)
	me := workspaceTestResponse[workspace.User](t, workspaceTestJSON(router, "GET", "/api/v1/me", tenant, ""), 200)
	if me.UsedBytes != int64(len(original)) {
		t.Fatalf("dynamic transforms changed billed usage: %d", me.UsedBytes)
	}

	variant := workspaceTestResponse[workspaceAsset](t, workspaceTestJSON(router, "POST", detail+"/variants", tenant,
		`{"folder":"exports/small","transform":{"width":32,"format":"jpeg"}}`), 201)
	variantPath := workspaceTestPath(t, variant.URL)
	checkWorkspaceImage(t, workspaceTestJSON(router, "GET", variantPath, nil, ""), "image/jpeg", 32, 16)
	listing := workspaceTestResponse[struct{ Items []workspaceAsset }](t, workspaceTestJSON(router, "GET", "/api/v1/images?folder=exports%2Fsmall&include_variants=true", tenant, ""), 200)
	if len(listing.Items) != 1 || listing.Items[0].ID != variant.ID || listing.Items[0].ImageID != image.ID {
		t.Fatalf("variant missing from destination folder: %+v", listing.Items)
	}
	me = workspaceTestResponse[workspace.User](t, workspaceTestJSON(router, "GET", "/api/v1/me", tenant, ""), 200)
	if me.UsedBytes != int64(len(original))+variant.SizeBytes {
		t.Fatalf("saved variant missing from billed usage: %d", me.UsedBytes)
	}
	workspaceTestStatus(t, workspaceTestJSON(router, "PUT", "/api/v1/settings", admin, `{"public_prefix":"/i/new"}`), 400)
	workspaceTestStatus(t, workspaceTestJSON(router, "PUT", "/api/v1/settings", admin, `{"public_prefix":"/cdn/photos"}`), 200)

	reopened := openWorkspaceForTest(t, runtimeDir, cfg.Path)
	router = workspaceTestRouter(cfg, reopened, legacyDir)
	for _, path := range []string{href, "/cdn/photos/" + image.ID, variantPath, "/i/legacy.png"} {
		workspaceTestStatus(t, workspaceTestJSON(router, "GET", path, nil, ""), 200)
	}
	request := httptest.NewRequest("GET", "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	workspaceTestStatus(t, runWorkspaceRequest(router, request), 200)
	workspaceTestStatus(t, workspaceTestJSON(router, "PATCH", "/api/v1/users/"+studio.ID, admin, `{"disabled":true}`), 200)
	workspaceTestStatus(t, runWorkspaceRequest(router, request), 401)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", detail, tenant, ""), 401)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", href, nil, ""), 200)
	workspaceTestStatus(t, workspaceTestJSON(router, "DELETE", detail, admin, ""), 204)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", href, nil, ""), 404)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", variantPath, nil, ""), 404)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", "/i/legacy.png", nil, ""), 200)
	schema := workspaceTestResponse[struct{ OpenAPI string }](t, workspaceTestJSON(router, "GET", "/api/v1/openapi.json", nil, ""), 200)
	if !strings.HasPrefix(schema.OpenAPI, "3.1.") {
		t.Fatalf("unexpected OpenAPI document version %q", schema.OpenAPI)
	}
}

func TestWorkspaceConcurrentUploadsRespectQuotaAndDeletionReleasesIt(t *testing.T) {
	cfg := workspaceTestConfig()
	cfg.Path = "/media/"
	store := openWorkspaceForTest(t, t.TempDir(), cfg.Path)
	if settings := store.Settings(); settings.PublicPrefix == "/media" {
		t.Fatal("default workspace prefix shadows a legacy image route")
	}
	router := workspaceTestRouter(cfg, store, t.TempDir())
	original := workspaceTestPNG(t)
	user, err := store.CreateUser(workspace.Identity{ID: workspace.AdminID}, cfg.User, workspace.CreateUser{
		Username: "uploader", Password: "uploader-password", QuotaBytes: int64(len(original)),
	})
	if err != nil {
		t.Fatal(err)
	}
	session := workspaceTestLogin(t, router, user.Username, "uploader-password")
	token := workspaceTestResponse[struct{ Token string }](t, workspaceTestJSON(router, "POST", "/api/v1/tokens", session, ""), 201).Token
	const concurrent = 6
	responses := make(chan *httptest.ResponseRecorder, concurrent)
	var group sync.WaitGroup
	for i := 0; i < concurrent; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			responses <- workspaceTestUpload(router, token, "", original)
		}()
	}
	group.Wait()
	close(responses)
	var created []workspaceAsset
	for result := range responses {
		switch result.Code {
		case 201:
			created = append(created, workspaceTestResponse[workspaceAsset](t, result, 201))
		case 409:
			problem := workspaceTestResponse[workspaceProblem](t, result, 409)
			if result.Header().Get("Content-Type") != "application/problem+json" || problem.Code != "quota_exceeded" || problem.QuotaError == nil ||
				problem.UsedBytes != int64(len(original)) || problem.QuotaBytes != int64(len(original)) {
				t.Fatalf("unexpected quota response: %s", result.Body)
			}
		default:
			t.Fatalf("unexpected upload response: %d %s", result.Code, result.Body)
		}
	}
	if len(created) != 1 {
		t.Fatalf("committed %d uploads at a one-image quota", len(created))
	}
	resource := "/api/v1/images/" + created[0].ID
	workspaceTestStatus(t, workspaceTestJSON(router, "POST", resource+"/variants", session, `{"transform":{"format":"jpeg"}}`), 409)
	workspaceTestStatus(t, workspaceTestJSON(router, "DELETE", resource, session, ""), 204)
	workspaceTestStatus(t, workspaceTestUpload(router, token, "", original), 201)
}

func TestWorkspaceKeepsLegacyAPINamespaceImages(t *testing.T) {
	cfg := workspaceTestConfig()
	cfg.Path = "/api/v1/"
	// A PHP migration can leave the administrator on its SHA-256 password.
	cfg.Password = service.NewLegacyPasswordHasher().GetHash("legacy-password")
	legacyDir := t.TempDir()
	original := workspaceTestPNG(t)
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy.png"), original, 0600); err != nil {
		t.Fatal(err)
	}
	router := workspaceTestRouter(cfg, openWorkspaceForTest(t, t.TempDir(), cfg.Path), legacyDir)
	got := workspaceTestJSON(router, "GET", "/api/v1/legacy.png", nil, "")
	if got.Code != 200 || !bytes.Equal(got.Body.Bytes(), original) {
		t.Fatalf("legacy image failed after registering workspace routes: HTTP %d", got.Code)
	}
	admin := workspaceTestLogin(t, router, cfg.User, "legacy-password")
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", "/api/workspace/v1/me", admin, ""), 200)
	schema := workspaceTestResponse[struct{ Paths map[string]json.RawMessage }](t,
		workspaceTestJSON(router, "GET", "/api/workspace/v1/openapi.json", nil, ""), 200)
	if _, exists := schema.Paths["/api/workspace/v1/images"]; !exists {
		t.Fatal("OpenAPI document missing the active workspace API prefix")
	}
}

func workspaceTestConfig() *config.Config {
	return &config.Config{
		User: "legacyadmin", Password: service.HashPassword("legacy-password"),
		Domain: "http://images.example.test", Path: "/i/", MustLogin: 1, MaxSize: 1 << 20, Extensions: "png,jpg,jpeg,webp",
	}
}

func openWorkspaceForTest(t *testing.T, dir, legacyPath string) *workspace.Store {
	t.Helper()
	store, err := workspace.Open(dir, legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func workspaceTestRouter(cfg *config.Config, store *workspace.Store, legacyDir string) *gin.Engine {
	router := gin.New()
	router.Static(strings.TrimRight(cfg.Path, "/"), legacyDir)
	router.POST("/api/login", AdminLoginAPI(cfg, store))
	router.POST("/app/upload", middleware.LegacyUploadOnly, middleware.CheckLogin(cfg), Upload(cfg))
	RegisterWorkspaceRoutes(router, cfg, store)
	return router
}

func workspaceTestLogin(t *testing.T, router http.Handler, username, password string) *http.Cookie {
	t.Helper()
	form := url.Values{"user": {username}, "password": {password}}
	request := httptest.NewRequest("POST", "/api/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	result := runWorkspaceRequest(router, request)
	workspaceTestStatus(t, result, 200)
	for _, cookie := range result.Result().Cookies() {
		if cookie.Name == "session" {
			return cookie
		}
	}
	t.Fatal("login response has no session cookie")
	return nil
}

func workspaceTestPNG(t *testing.T) []byte {
	t.Helper()
	image := image.NewNRGBA(image.Rect(0, 0, 96, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 96; x++ {
			image.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 2), G: uint8(y * 5), B: 42, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func workspaceTestUpload(router http.Handler, token, folder string, data []byte) *httptest.ResponseRecorder {
	body := new(bytes.Buffer)
	form := multipart.NewWriter(body)
	file, _ := form.CreateFormFile("file", "source.png")
	_, _ = file.Write(data)
	_ = form.WriteField("folder", folder)
	_ = form.Close()
	req := httptest.NewRequest("POST", "/api/v1/images", body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	return runWorkspaceRequest(router, req)
}

func workspaceTestJSON(router http.Handler, method, path string, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return runWorkspaceRequest(router, req)
}

func runWorkspaceRequest(router http.Handler, request *http.Request) *httptest.ResponseRecorder {
	result := httptest.NewRecorder()
	router.ServeHTTP(result, request)
	return result
}

func workspaceTestStatus(t *testing.T, result *httptest.ResponseRecorder, status int) {
	t.Helper()
	if result.Code != status {
		t.Fatalf("HTTP %d, want %d: %s", result.Code, status, result.Body)
	}
}

func workspaceTestResponse[T any](t *testing.T, result *httptest.ResponseRecorder, status int) T {
	t.Helper()
	workspaceTestStatus(t, result, status)
	var data T
	if err := json.Unmarshal(result.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func workspaceTestPath(t *testing.T, href string) string {
	t.Helper()
	parsed, err := url.Parse(href)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Path
}

func checkWorkspaceImage(t *testing.T, result *httptest.ResponseRecorder, contentType string, width, height int) {
	t.Helper()
	workspaceTestStatus(t, result, 200)
	decoded, _, err := image.DecodeConfig(bytes.NewReader(result.Body.Bytes()))
	if err != nil || result.Header().Get("Content-Type") != contentType || decoded.Width != width || decoded.Height != height {
		t.Fatalf("expected %s %dx%d, got %s %+v (%v)", contentType, width, height, result.Header().Get("Content-Type"), decoded, err)
	}
}
