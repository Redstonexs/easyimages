package service

import (
	"easyimage/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetImageInfoIncludesOriginalName(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(imageDir, "stored.jpg"), []byte("x"), 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	SaveImageMetadata("/i/2026/06/02/stored.jpg", "Original Photo.jpg", 1, "web", time.Date(2026, 6, 2, 1, 2, 3, 0, time.UTC))

	info, err := GetImageInfo("/i/2026/06/02/stored.jpg", &config.Config{Path: "/i/", Domain: "https://img.example.com"})
	if err != nil {
		t.Fatalf("GetImageInfo() error = %v", err)
	}
	if info["originalName"] != "Original Photo.jpg" {
		t.Fatalf("originalName = %q", info["originalName"])
	}
	if info["displayName"] != "Original Photo.jpg" {
		t.Fatalf("displayName = %q", info["displayName"])
	}
	if info["storedName"] != "stored.jpg" {
		t.Fatalf("storedName = %q", info["storedName"])
	}
}
