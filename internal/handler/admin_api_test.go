package handler

import (
	"bytes"
	"easyimage/config"
	"easyimage/internal/service"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAdminConfigFromConfigDoesNotExposeSecrets(t *testing.T) {
	cfg := &config.Config{
		Title:              "EasyImage",
		Password:           "secret-password-hash",
		User:               "admin",
		HideKey:            "hide-secret",
		TurnstileSiteKey:   "site-key",
		TurnstileSecretKey: "secret-key",
		RecaptchaSecretKey: "recaptcha-secret",
	}

	payload := adminConfigFromConfig(cfg)
	if payload.TurnstileSiteKey != "site-key" {
		t.Fatalf("TurnstileSiteKey = %q", payload.TurnstileSiteKey)
	}
	if !payload.TurnstileSecretSet || !payload.RecaptchaSecretSet {
		t.Fatal("secret presence flags were not set")
	}
	if payload.SiteIcon != cfg.SiteIcon {
		t.Fatalf("SiteIcon = %q, want %q", payload.SiteIcon, cfg.SiteIcon)
	}
}

func TestAdminSiteIconUploadRejectsUnsafeSVG(t *testing.T) {
	tmp := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	req := siteIconUploadRequest(t, "evil.svg", `<svg onload="alert(1)"></svg>`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	AdminSiteIconUploadAPI(&config.Config{Title: "EasyImage", SiteIcon: "/favicon.ico"})(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if _, err := os.Stat(filepath.Join("config", "site-icon", "favicon.svg")); !os.IsNotExist(err) {
		t.Fatalf("unsafe SVG was saved, stat error = %v", err)
	}
}

func TestAdminSiteIconUploadSavesIconAndConfig(t *testing.T) {
	tmp := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}

	cfg := &config.Config{Title: "EasyImage", SiteIcon: "/favicon.ico"}
	req := siteIconUploadRequest(t, "favicon.png", "png-bytes")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	AdminSiteIconUploadAPI(cfg)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(cfg.SiteIcon, "/favicon.ico?v=") {
		t.Fatalf("SiteIcon = %q, want cache-busted favicon URL", cfg.SiteIcon)
	}
	if _, err := os.Stat(filepath.Join("config", "site-icon", "favicon.png")); err != nil {
		t.Fatalf("uploaded icon was not saved: %v", err)
	}
	if _, err := os.Stat(filepath.Join("config", "config.json")); err != nil {
		t.Fatalf("config was not saved: %v", err)
	}
}

func siteIconUploadRequest(t *testing.T, filename, content string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("icon", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/admin/api/site-icon", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestAdminURLListPayloadUsesThumbnails(t *testing.T) {
	tmp := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	imageDir := filepath.Join("i", "2026", "06", "02")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("create image dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(imageDir, "large.jpg"), []byte("x"), 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/api/urllist?path=/i/&page=1&page_size=50", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	payload, err := adminURLListPayload(c, &config.Config{Path: "/i/", Domain: "https://img.example.com"})
	if err != nil {
		t.Fatalf("adminURLListPayload() error = %v", err)
	}
	if len(payload.Files) != 1 {
		t.Fatalf("Files length = %d, want 1", len(payload.Files))
	}
	file := payload.Files[0]
	if file.ThumbURL != "/app/thumb?img=/i/2026/06/02/large.jpg" {
		t.Fatalf("ThumbURL = %q", file.ThumbURL)
	}
	if file.URL != "https://img.example.com/i/2026/06/02/large.jpg" {
		t.Fatalf("URL = %q", file.URL)
	}
	if file.Path != "/i/2026/06/02/large.jpg" {
		t.Fatalf("Path = %q", file.Path)
	}
}

func TestAdminURLListPayloadSearchesOriginalFileName(t *testing.T) {
	tmp := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	imageDir := filepath.Join("i", "2026", "06", "02")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("create image dir: %v", err)
	}
	for _, name := range []string{"stored.jpg", "other.jpg"} {
		if err := os.WriteFile(filepath.Join(imageDir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	service.SaveImageMetadata("/i/2026/06/02/stored.jpg", "Holiday Beach.jpg", 1, "web", time.Date(2026, 6, 2, 1, 2, 3, 0, time.UTC))

	req := httptest.NewRequest("GET", "/admin/api/urllist?path=/i/&q=beach&page=1&page_size=50", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	payload, err := adminURLListPayload(c, &config.Config{Path: "/i/", Domain: "https://img.example.com"})
	if err != nil {
		t.Fatalf("adminURLListPayload() error = %v", err)
	}
	if payload.Total != 1 {
		t.Fatalf("Total = %d, want 1", payload.Total)
	}
	if len(payload.Files) != 1 {
		t.Fatalf("Files length = %d, want 1", len(payload.Files))
	}
	if payload.Files[0].OriginalName != "Holiday Beach.jpg" {
		t.Fatalf("OriginalName = %q", payload.Files[0].OriginalName)
	}
	if payload.Query != "beach" {
		t.Fatalf("Query = %q", payload.Query)
	}
}

func TestAdminURLListPayloadSearchesStoredPath(t *testing.T) {
	tmp := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	imageDir := filepath.Join("i", "2026", "06", "02")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("create image dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(imageDir, "stored-name.jpg"), []byte("x"), 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/api/urllist?path=/i/&q=stored-name&page=1&page_size=50", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	payload, err := adminURLListPayload(c, &config.Config{Path: "/i/", Domain: "https://img.example.com"})
	if err != nil {
		t.Fatalf("adminURLListPayload() error = %v", err)
	}
	if payload.Total != 1 || len(payload.Files) != 1 || payload.Files[0].Name != "2026/06/02/stored-name.jpg" {
		t.Fatalf("payload = %+v", payload)
	}
}
