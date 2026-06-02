package service

import (
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
