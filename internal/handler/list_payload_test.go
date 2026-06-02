package handler

import (
	"easyimage/config"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestImageListPayloadBuildsFrontendGalleryData(t *testing.T) {
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

	date := "2026/06/02/"
	imageDir := filepath.Join("i", "2026", "06", "02")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("create image dir: %v", err)
	}
	for _, name := range []string{"first.jpg", "second.png", "note.txt"} {
		if err := os.WriteFile(filepath.Join(imageDir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	req := httptest.NewRequest("GET", "/api/list?date="+date+"&search=jpg", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	payload, err := imageListPayload(c, &config.Config{
		Title:      "EasyImage",
		Domain:     "https://img.example.com",
		Path:       "/i/",
		ListDate:   10,
		ListNumber: 20,
	})
	if err != nil {
		t.Fatalf("imageListPayload() error = %v", err)
	}
	if payload.Total != 1 {
		t.Fatalf("Total = %d, want 1", payload.Total)
	}
	if len(payload.Files) != 1 {
		t.Fatalf("Files length = %d, want 1", len(payload.Files))
	}
	file := payload.Files[0]
	if file.Name != "first.jpg" {
		t.Fatalf("Name = %q, want first.jpg", file.Name)
	}
	if file.URL != "https://img.example.com/i/2026/06/02/first.jpg" {
		t.Fatalf("URL = %q", file.URL)
	}
	if file.InfoURL != "/app/info?img=/i/2026/06/02/first.jpg" {
		t.Fatalf("InfoURL = %q", file.InfoURL)
	}
}

func TestImageListPayloadRejectsUnsafeSearch(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/list?search=../jpg", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	_, err := imageListPayload(c, &config.Config{Path: "/i/", ListDate: 1, ListNumber: 20})
	if err == nil {
		t.Fatal("imageListPayload() error = nil, want unsafe search error")
	}
}

func TestImageListPayloadRejectsUnsafeDate(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/list?date=../", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	_, err := imageListPayload(c, &config.Config{Path: "/i/", ListDate: 1, ListNumber: 20})
	if err == nil {
		t.Fatal("imageListPayload() error = nil, want unsafe date error")
	}
}
