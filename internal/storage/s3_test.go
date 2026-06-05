package storage

import (
	"testing"

	"easyimage/config"
)

func TestObjectKeyUsesPrefixAndStripsStorageRoot(t *testing.T) {
	source := config.StorageSourceConfig{ID: "s3-main", Type: "s3", S3Prefix: "uploads"}
	key, err := ObjectKey(source, "/i/2026/06/05/photo.jpg")
	if err != nil {
		t.Fatalf("ObjectKey() error = %v", err)
	}
	if key != "uploads/2026/06/05/photo.jpg" {
		t.Fatalf("ObjectKey() = %q", key)
	}
}

func TestPublicURLUsesConfiguredBase(t *testing.T) {
	source := config.StorageSourceConfig{PublicBaseURL: "https://cdn.example.com/images/"}
	url := PublicURL(source, "2026/06/05/photo.jpg")
	if url != "https://cdn.example.com/images/2026/06/05/photo.jpg" {
		t.Fatalf("PublicURL() = %q", url)
	}
}
