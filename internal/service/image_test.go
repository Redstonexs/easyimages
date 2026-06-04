package service

import (
	"easyimage/config"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("FAKE_CWEBP") == "1" {
		runFakeCwebp()
		return
	}
	os.Exit(m.Run())
}

func TestConvertToWebPAcceptsAbsoluteUploadPath(t *testing.T) {
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
	prependFakeCwebp(t)

	imgPath := filepath.Join(tmp, "i", "2026", "05", "12", "photo.jpg")
	if err := os.MkdirAll(filepath.Dir(imgPath), 0755); err != nil {
		t.Fatalf("create image dir: %v", err)
	}
	if err := os.WriteFile(imgPath, []byte("fake jpg"), 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	cfg := &config.Config{Path: "/i/", WebpQuality: 80}
	if err := ConvertToWebP(imgPath, cfg); err != nil {
		t.Fatalf("ConvertToWebP() error = %v", err)
	}

	webpPath := filepath.Join(tmp, "i", "webp", "2026", "05", "12", "photo.webp")
	if _, err := os.Stat(webpPath); err != nil {
		t.Fatalf("expected webp file at %s: %v", webpPath, err)
	}
}

func TestGetWebPURLReturnsExistingWebPPath(t *testing.T) {
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

	webpPath := filepath.Join("i", "webp", "2026", "06", "02", "photo.webp")
	if err := os.MkdirAll(filepath.Dir(webpPath), 0755); err != nil {
		t.Fatalf("create webp dir: %v", err)
	}
	if err := os.WriteFile(webpPath, []byte("fake webp"), 0644); err != nil {
		t.Fatalf("write webp: %v", err)
	}

	cfg := &config.Config{Domain: "https://img.example.com", Path: "/i/"}
	got := GetWebPURL("/i/2026/06/02/photo.jpg", cfg)
	want := "https://img.example.com/i/webp/2026/06/02/photo.webp"
	if got != want {
		t.Fatalf("GetWebPURL() = %q, want %q", got, want)
	}
}

func TestGetWebPURLRejectsTraversalPath(t *testing.T) {
	cfg := &config.Config{Domain: "https://img.example.com", Path: "/i/"}
	if got := GetWebPURL("/i/2026/../../config/config.json", cfg); got != "" {
		t.Fatalf("GetWebPURL() = %q, want empty string", got)
	}
}

func TestStartImagePostProcessDoesNotBlockWhenQueueIsFull(t *testing.T) {
	originalQueue := imageProcessQueue
	imageProcessQueue = make(chan imageProcessJob)
	t.Cleanup(func() {
		imageProcessQueue = originalQueue
	})

	done := make(chan struct{})
	go func() {
		StartImagePostProcess(filepath.Join(t.TempDir(), "photo.jpg"), &config.Config{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("StartImagePostProcess blocked when the queue was unavailable")
	}
}

func prependFakeCwebp(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	name := "cwebp"
	if filepath.Ext(os.Args[0]) == ".exe" {
		name += ".exe"
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	fakePath := filepath.Join(binDir, name)
	if err := copyFile(fakePath, exe); err != nil {
		t.Fatalf("install fake cwebp: %v", err)
	}
	t.Setenv("FAKE_CWEBP", "1")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func runFakeCwebp() {
	var out string
	for i := 1; i < len(os.Args)-1; i++ {
		if os.Args[i] == "-o" {
			out = os.Args[i+1]
			break
		}
	}
	if out == "" {
		os.Exit(2)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(out, []byte("fake webp"), 0644); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
