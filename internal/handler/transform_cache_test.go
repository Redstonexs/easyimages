package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"easyimage/internal/workspace"
)

func TestTransformCacheBoundsAndExpiry(t *testing.T) {
	cache := newTransformCache()
	now := time.Now()
	data := make([]byte, maxCachedTransformSize)
	for i := 0; i <= maxTransformCacheBytes/maxCachedTransformSize; i++ {
		cache.put(strconv.Itoa(i), data, now)
	}
	if cache.bytes != maxTransformCacheBytes || cache.get("0", now) != nil {
		t.Fatalf("cache did not evict the oldest entry at the byte limit")
	}
	if cache.get("1", now) == nil || cache.get("1", now.Add(transformCacheTTL)) != nil {
		t.Fatalf("cache did not expire the entry after its TTL")
	}
	cache.put("large", make([]byte, maxCachedTransformSize+1), now)
	if cache.get("large", now) != nil {
		t.Fatalf("oversize response was cached")
	}
}

func TestWorkspaceHotlinkResponsesBypassSharedCaches(t *testing.T) {
	cfg := workspaceTestConfig()
	cfg.HotlinkProtect = 1
	store := openWorkspaceForTest(t, t.TempDir(), cfg.Path)
	original := workspaceTestPNG(t)
	asset, err := store.Upload(workspace.Identity{ID: workspace.AdminID}, "", "", "source.png", bytes.NewReader(original), int64(len(original)), cfg.MaxSize)
	if err != nil {
		t.Fatal(err)
	}
	router := workspaceTestRouter(cfg, store, t.TempDir())
	path := "/media/" + asset.ID + "?width=24"
	get := func(etag string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("If-None-Match", etag)
		return runWorkspaceRequest(router, request)
	}
	result := get("")
	workspaceTestStatus(t, result, 200)
	for _, response := range []*httptest.ResponseRecorder{result, get(""), get(result.Header().Get("ETag"))} {
		if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("protected response exposed a shared cache policy: %q", got)
		}
	}
}
