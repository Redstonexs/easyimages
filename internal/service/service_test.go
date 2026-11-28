package service

import (
	"easyimage/config"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetFileListLimited(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"001.jpg", "002.png", "003.gif", "note.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0755); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}

	tests := []struct {
		name      string
		pattern   string
		sortOrder int
		limit     int
		wantFiles []string
		wantTotal int
	}{
		{
			name:      "limited ascending keeps total count",
			pattern:   filepath.Join(dir, "*.jpg"),
			sortOrder: 0,
			limit:     1,
			wantFiles: []string{"001.jpg"},
			wantTotal: 1,
		},
		{
			name:      "limited all image-like glob",
			pattern:   filepath.Join(dir, "*.*"),
			sortOrder: 0,
			limit:     2,
			wantFiles: []string{"001.jpg", "002.png"},
			wantTotal: 4,
		},
		{
			name:      "descending returns newest names first",
			pattern:   filepath.Join(dir, "*.*"),
			sortOrder: 1,
			limit:     2,
			wantFiles: []string{"note.txt", "003.gif"},
			wantTotal: 4,
		},
		{
			name:      "zero limit returns all",
			pattern:   filepath.Join(dir, "*.*"),
			sortOrder: 0,
			limit:     0,
			wantFiles: []string{"001.jpg", "002.png", "003.gif", "note.txt"},
			wantTotal: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFiles, gotTotal := GetFileListLimited(tt.pattern, tt.sortOrder, tt.limit)
			if gotTotal != tt.wantTotal {
				t.Fatalf("total = %d, want %d", gotTotal, tt.wantTotal)
			}
			if !reflect.DeepEqual(gotFiles, tt.wantFiles) {
				t.Fatalf("files = %#v, want %#v", gotFiles, tt.wantFiles)
			}
		})
	}
}

func TestDeleteFileWithConfigCleansCompanionWebPAndThumb(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, rel := range []string{
		"i/2026/05/01/a.png",
		"i/webp/2026/05/01/a.webp",
		"i/cache/2026_05_01_a.png",
	} {
		if err := os.MkdirAll(filepath.Dir(rel), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rel, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{
		Domain: "https://img.example.com",
		Path:   "/i/",
	}
	if err := DeleteFileWithConfig("https://img.example.com/i/2026/05/01/a.png", cfg); err != nil {
		t.Fatalf("DeleteFileWithConfig() error = %v", err)
	}

	for _, rel := range []string{
		"i/2026/05/01/a.png",
		"i/webp/2026/05/01/a.webp",
		"i/cache/2026_05_01_a.png",
	} {
		if _, err := os.Stat(rel); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be deleted, stat err = %v", rel, err)
		}
	}
}

func TestCollectDirStatsSkipsDotfilesAndNonImages(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		".gitkeep",
		"2026/05/01/.hidden.jpg",
		"2026/05/01/note.txt",
		"2026/05/01/photo.png",
		"webp/2026/05/01/photo.webp",
	} {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("1234"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	stats := CollectDirStats(dir)
	if stats.TotalFiles != 1 {
		t.Fatalf("TotalFiles = %d, want 1", stats.TotalFiles)
	}
	if stats.TotalSize != 4 {
		t.Fatalf("TotalSize = %d, want 4", stats.TotalSize)
	}
	if stats.ByExt[".png"] != 1 || len(stats.ByExt) != 1 {
		t.Fatalf("ByExt = %#v, want only .png:1", stats.ByExt)
	}
	if stats.DailyCounts["2026/05/01"] != 1 {
		t.Fatalf("DailyCounts = %#v, want 2026/05/01:1", stats.DailyCounts)
	}
}
