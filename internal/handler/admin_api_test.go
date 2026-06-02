package handler

import (
	"easyimage/config"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
