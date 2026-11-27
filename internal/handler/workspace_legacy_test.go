package handler

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"easyimage/internal/workspace"
)

func TestWorkspaceIncludesLegacyImagesAndFoldersForAdministrator(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := workspaceTestConfig()
	cfg.ListDate = 1
	original := workspaceTestPNG(t)
	rendition := []byte("legacy WebP fixture")
	writeFile := func(name string, contents []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("i/2024/02/03/old.png", original)
	writeFile("i/2024/02/03/without-webp.png", original)
	writeFile("i/webp/2024/02/03/old.webp", rendition)
	writeFile("i/webp/2024/02/03/orphan.webp", rendition)
	writeFile("i/webp/2024/02/03/without-webp.webp", nil)
	writeFile("i/cache/internal.png", original)
	writeFile("i/legacy%folder/a #b.png", original)
	writeFile("i/webp/legacy%folder/a #b.webp", rendition)
	writeFile("admin/logs/metadata/2023-06.json", []byte(`{
		"/i/remote/archives/avatar.webp": {
			"path": "/i/remote/archives/avatar.webp",
			"original_name": "avatar.webp",
			"size": 500,
			"uploaded_at": "2023-06-14 12:00:00",
			"storage_type": "s3",
			"url": "https://bucket.example.test/remote/archives/avatar.webp"
		}
	}`))
	if err := os.MkdirAll("i/empty", 0755); err != nil {
		t.Fatal(err)
	}

	store := openWorkspaceForTest(t, "admin/logs/workspace", cfg.Path)
	router := workspaceTestRouter(cfg, store, "i")
	admin := workspaceTestLogin(t, router, cfg.User, "legacy-password")
	user := workspaceTestResponse[workspace.User](t, workspaceTestJSON(router, "POST", "/api/v1/users", admin,
		`{"username":"member","password":"member-password"}`), 201)
	tenant := workspaceTestLogin(t, router, user.Username, "member-password")
	token := workspaceTestResponse[struct{ Token string }](t, workspaceTestJSON(router, "POST", "/api/v1/tokens", admin, ""), 201).Token

	type foldersResponse struct{ Items []workspaceFolder }
	type imagesResponse struct {
		Items      []workspaceAsset
		NextCursor string `json:"next_cursor"`
	}
	const folders = "/api/v1/folders?include_legacy=true"
	const images = "/api/v1/images?include_legacy=true"
	folderList := workspaceTestResponse[foldersResponse](t, workspaceTestJSON(router, "GET", folders, admin, ""), 200)
	var paths []string
	for _, folder := range folderList.Items {
		paths = append(paths, folder.Path)
		if folder.BrowseOnly != (folder.Path == "legacy%folder") {
			t.Fatalf("unexpected writable state for %q", folder.Path)
		}
	}
	want := []string{"2024", "2024/02", "2024/02/03", "empty", "legacy%folder", "remote", "remote/archives"}
	if !slices.Equal(paths, want) {
		t.Fatalf("got folders %v, want %v", paths, want)
	}

	uploaded := workspaceTestResponse[workspaceAsset](t, workspaceTestUpload(router, token, "2024/02/03", original), 201)
	if uploaded.Legacy || uploaded.Folder != "2024/02/03" || workspaceTestPath(t, uploaded.URL) != "/media/"+uploaded.ID {
		t.Fatalf("unexpected upload into a legacy folder: %+v", uploaded)
	}
	if _, err := exec.LookPath("cwebp"); err == nil {
		href, err := url.Parse(uploaded.WebPURL)
		if err != nil || href.Path != "/media/"+uploaded.ID || href.Query().Get("format") != "webp" {
			t.Fatalf("bad WebP link %q: %v", uploaded.WebPURL, err)
		}
		checkWorkspaceImage(t, workspaceTestJSON(router, "GET", href.RequestURI(), nil, ""), "image/webp", 96, 48)
	}
	variant := workspaceTestResponse[workspaceAsset](t, workspaceTestJSON(router, "POST", "/api/v1/images/"+uploaded.ID+"/variants", admin,
		`{"folder":"empty","transform":{"width":24,"format":"png"}}`), 201)
	if variant.Folder != "empty" {
		t.Fatalf("variant went to %q", variant.Folder)
	}

	page := workspaceTestResponse[imagesResponse](t, workspaceTestJSON(router, "GET", images+"&folder=2024/02/03&limit=1", admin, ""), 200)
	if len(page.Items) != 1 || page.Items[0].ID != uploaded.ID || page.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", page)
	}
	page = workspaceTestResponse[imagesResponse](t, workspaceTestJSON(router, "GET", images+"&folder=2024/02/03&cursor="+page.NextCursor, admin, ""), 200)
	if len(page.Items) != 3 || page.NextCursor != "" {
		t.Fatalf("unexpected second page: %+v", page)
	}
	var old workspaceAsset
	for _, item := range page.Items {
		if !item.Legacy || (item.WebPURL == "") != (item.Filename == "without-webp.png") {
			t.Fatalf("unexpected legacy WebP link: %+v", item)
		}
		if item.Filename == "old.png" {
			old = item
		}
	}
	if old.URL != cfg.Domain+"/i/2024/02/03/old.png" || old.WebPURL != cfg.Domain+"/i/webp/2024/02/03/old.webp" {
		t.Fatalf("legacy links changed: %+v", old)
	}
	for href, expected := range map[string][]byte{old.URL: original, old.WebPURL: rendition} {
		got := workspaceTestJSON(router, "GET", workspaceTestPath(t, href), nil, "")
		if got.Code != 200 || !bytes.Equal(got.Body.Bytes(), expected) {
			t.Fatalf("legacy link changed the served file: %s (HTTP %d)", href, got.Code)
		}
	}
	detail := "/api/v1/images/" + old.ID
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", detail, admin, ""), 200)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", detail, tenant, ""), 404)
	workspaceTestStatus(t, workspaceTestJSON(router, "DELETE", detail, admin, ""), 404)
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", images+"&folder=..%2Fconfig", admin, ""), 400)

	out := workspaceTestResponse[imagesResponse](t, workspaceTestJSON(router, "GET", images+"&folder=legacy%25folder", admin, ""), 200)
	if len(out.Items) != 1 || out.Items[0].URL != cfg.Domain+"/i/legacy%25folder/a%20%23b.png" ||
		out.Items[0].WebPURL != cfg.Domain+"/i/webp/legacy%25folder/a%20%23b.webp" {
		t.Fatalf("legacy filename was not URL-escaped: %+v", out)
	}
	remote := workspaceTestResponse[imagesResponse](t, workspaceTestJSON(router, "GET", images+"&folder=remote/archives", admin, ""), 200)
	if len(remote.Items) != 1 || remote.Items[0].URL != "https://bucket.example.test/remote/archives/avatar.webp" || remote.Items[0].URL != remote.Items[0].WebPURL {
		t.Fatalf("old S3 image missing: %+v", remote)
	}
	all := workspaceTestResponse[imagesResponse](t, workspaceTestJSON(router, "GET", images, admin, ""), 200)
	if len(all.Items) != 6 {
		t.Fatalf("listing includes internal or duplicate renditions: %+v", all)
	}
	managed := workspaceTestResponse[imagesResponse](t, workspaceTestJSON(router, "GET", "/api/v1/images", admin, ""), 200)
	if len(managed.Items) != 1 || managed.Items[0].ID != uploaded.ID {
		t.Fatalf("workspace-only listing changed: %+v", managed)
	}
	for target, cookie := range map[string]*http.Cookie{"": tenant, "&owner_id=" + user.ID: admin} {
		got := workspaceTestResponse[imagesResponse](t, workspaceTestJSON(router, "GET", images+target, cookie, ""), 200)
		dirs := workspaceTestResponse[foldersResponse](t, workspaceTestJSON(router, "GET", folders+target, cookie, ""), 200)
		if len(got.Items) != 0 || len(dirs.Items) != 0 {
			t.Fatalf("legacy inventory leaked to the tenant workspace")
		}
	}

	// Verify direct ID lookup semantics for orphan WebP, paired WebP (must not be exposed),
	// S3 legacy items, and invalid hex IDs.
	orphanID := legacyAssetID("/i/webp/2024/02/03/orphan.webp")
	orphanDetail := workspaceTestResponse[struct{ Image workspaceAsset }](t, workspaceTestJSON(router, "GET", "/api/v1/images/"+orphanID, admin, ""), 200)
	if orphanDetail.Image.ID != orphanID || orphanDetail.Image.Filename != "orphan.webp" || !orphanDetail.Image.Legacy {
		t.Fatalf("unexpected orphan detail: %+v", orphanDetail.Image)
	}
	pairedWebPID := legacyAssetID("/i/webp/2024/02/03/old.webp")
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", "/api/v1/images/"+pairedWebPID, admin, ""), 404)
	s3ID := legacyAssetID("/i/remote/archives/avatar.webp")
	s3Detail := workspaceTestResponse[struct{ Image workspaceAsset }](t, workspaceTestJSON(router, "GET", "/api/v1/images/"+s3ID, admin, ""), 200)
	if s3Detail.Image.URL != "https://bucket.example.test/remote/archives/avatar.webp" {
		t.Fatalf("unexpected S3 legacy detail: %+v", s3Detail.Image)
	}
	workspaceTestStatus(t, workspaceTestJSON(router, "GET", "/api/v1/images/not-a-valid-32-char-legacy-id!!!", admin, ""), 404)

	// Verify hasLegacyFolder fast paths and internal-dir rejection.
	if !hasLegacyFolder(cfg, "empty") || !hasLegacyFolder(cfg, "2024/02/03") || !hasLegacyFolder(cfg, "remote") || !hasLegacyFolder(cfg, "remote/archives") {
		t.Fatal("hasLegacyFolder missed an existing legacy folder")
	}
	if hasLegacyFolder(cfg, "cache") || hasLegacyFolder(cfg, "webp") || hasLegacyFolder(cfg, "does-not-exist") {
		t.Fatal("hasLegacyFolder accepted an internal or nonexistent folder")
	}
}
