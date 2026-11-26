package handler

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"

	"easyimage/config"
	"easyimage/internal/workspace"

	"github.com/gin-gonic/gin"
)

func BenchmarkWorkspaceDynamicDelivery(b *testing.B) {
	source := image.NewRGBA(image.Rect(0, 0, 2400, 1600))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var input bytes.Buffer
	if err := jpeg.Encode(&input, source, &jpeg.Options{Quality: 90}); err != nil {
		b.Fatal(err)
	}
	const maxBytes = 5 << 20
	store, err := workspace.Open(b.TempDir(), "/i/")
	if err != nil {
		b.Fatal(err)
	}
	asset, err := store.Upload(workspace.Identity{ID: workspace.AdminID}, "", "", "photo.jpg", &input, int64(input.Len()), maxBytes)
	if err != nil {
		b.Fatal(err)
	}
	r := gin.New()
	RegisterWorkspaceRoutes(r, &config.Config{Path: "/i/", MaxSize: maxBytes}, store)
	request := httptest.NewRequest(http.MethodGet, "/media/"+asset.ID+"?width=480&format=jpeg", nil)
	warm := httptest.NewRecorder()
	r.ServeHTTP(warm, request)
	if warm.Code != http.StatusOK {
		b.Fatalf("warmup failed: %d %s", warm.Code, warm.Body)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			b.Fatalf("unexpected delivery status: %d", response.Code)
		}
	}
}
