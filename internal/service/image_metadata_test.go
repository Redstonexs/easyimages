package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImageMetadataSaveLoadAndDelete(t *testing.T) {
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

	uploadedAt := time.Date(2026, 6, 2, 12, 30, 0, 0, time.UTC)
	SaveImageMetadata("/i/2026/06/02/stored.jpg", "C:\\photos\\Summer Trip.JPG", 1234, "web", uploadedAt)

	metadata, ok := GetImageMetadata("/i/2026/06/02/stored.jpg")
	if !ok {
		t.Fatal("metadata not found")
	}
	if metadata.OriginalName != "Summer Trip.JPG" {
		t.Fatalf("OriginalName = %q", metadata.OriginalName)
	}
	if metadata.OriginalBase != "Summer Trip" {
		t.Fatalf("OriginalBase = %q", metadata.OriginalBase)
	}
	if metadata.StoredName != "stored.jpg" {
		t.Fatalf("StoredName = %q", metadata.StoredName)
	}
	if !MetadataMatchesQuery(metadata, "stored.jpg", "summer") {
		t.Fatal("metadata did not match original filename query")
	}

	if _, err := os.Stat(filepath.Join("admin", "logs", "metadata", "2026-06.json")); err != nil {
		t.Fatalf("metadata file missing: %v", err)
	}

	DeleteImageMetadata("/i/2026/06/02/stored.jpg")
	if _, ok := GetImageMetadata("/i/2026/06/02/stored.jpg"); ok {
		t.Fatal("metadata still exists after delete")
	}
}

func TestSaveImageMetadataRejectsTraversalPath(t *testing.T) {
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

	SaveImageMetadata("/i/2026/../../evil.jpg", "evil.jpg", 1, "web", time.Date(2026, 6, 2, 1, 2, 3, 0, time.UTC))

	if _, err := os.Stat(filepath.Join("admin", "logs", "metadata")); !os.IsNotExist(err) {
		t.Fatalf("metadata directory stat error = %v, want not exist", err)
	}
}

func TestSaveImageMetadataWithStoragePersistsS3Fields(t *testing.T) {
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

	SaveImageMetadataWithStorage("/i/2026/06/05/photo.jpg", "photo.jpg", 42, "web", time.Date(2026, 6, 5, 1, 2, 3, 0, time.UTC), "s3-main", "s3", "uploads/2026/06/05/photo.jpg", "https://cdn.example.com/uploads/2026/06/05/photo.jpg", "https://cdn.example.com/uploads/2026/06/05/photo.jpg")
	metadata, ok := GetImageMetadata("/i/2026/06/05/photo.jpg")
	if !ok {
		t.Fatal("metadata not found")
	}
	if metadata.StorageSource != "s3-main" || metadata.StorageType != "s3" || metadata.ObjectKey != "uploads/2026/06/05/photo.jpg" {
		t.Fatalf("metadata storage fields = %+v", metadata)
	}
}

func TestLoadImageMetadataForDateRejectsTraversalPath(t *testing.T) {
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

	if items := LoadImageMetadataForDate("../"); len(items) != 0 {
		t.Fatalf("LoadImageMetadataForDate returned %d items, want 0", len(items))
	}
}

func TestValidateURLPathRejectsNormalizedTraversal(t *testing.T) {
	if _, err := ValidateURLPath("/i/2026/../../evil.jpg", "/i/"); err == nil {
		t.Fatal("ValidateURLPath error = nil, want traversal error")
	}
}

func TestOriginalUploadNameStripsClientPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "windows path", in: `C:\photos\cat.jpg`, want: "cat.jpg"},
		{name: "unix path", in: "/tmp/dog.png", want: "dog.png"},
		{name: "plain name", in: "bird.webp", want: "bird.webp"},
		{name: "empty", in: "", want: "upload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OriginalUploadName(tt.in); got != tt.want {
				t.Fatalf("OriginalUploadName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
