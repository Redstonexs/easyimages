package main

import (
	"strings"
	"testing"
)

func TestViteAssetsTagsRendersImportsCSSAndEntry(t *testing.T) {
	assets := viteAssets{manifest: viteManifest{
		"web/src/entries/upload.ts": {
			File:    "assets/upload-abc.js",
			CSS:     []string{"assets/upload-def.css"},
			Imports: []string{"_app.js"},
		},
		"_app.js": {
			File: "assets/app-abc.js",
			CSS:  []string{"assets/app-def.css"},
		},
	}}

	tags := string(assets.Tags("web/src/entries/upload.ts"))
	for _, want := range []string{
		`<link rel="stylesheet" href="/public/dist/assets/app-def.css">`,
		`<link rel="modulepreload" href="/public/dist/assets/app-abc.js">`,
		`<link rel="stylesheet" href="/public/dist/assets/upload-def.css">`,
		`<script type="module" src="/public/dist/assets/upload-abc.js"></script>`,
	} {
		if !strings.Contains(tags, want) {
			t.Fatalf("tags missing %s in %s", want, tags)
		}
	}
}
