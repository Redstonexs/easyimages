package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"strings"
)

type viteChunk struct {
	File    string   `json:"file"`
	CSS     []string `json:"css"`
	Imports []string `json:"imports"`
}

type viteManifest map[string]viteChunk

type viteAssets struct {
	manifest viteManifest
}

func loadViteAssets(path string) viteAssets {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Vite manifest not found at %s; frontend assets will be skipped until npm run build is executed", path)
		return viteAssets{}
	}

	var manifest viteManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		log.Printf("Failed to parse Vite manifest %s: %v", path, err)
		return viteAssets{}
	}

	return viteAssets{manifest: manifest}
}

func (v viteAssets) Tags(entry string) template.HTML {
	if len(v.manifest) == 0 {
		return ""
	}

	chunk, ok := v.manifest[entry]
	if !ok {
		log.Printf("Vite manifest entry %q not found", entry)
		return ""
	}

	var b strings.Builder
	seenCSS := make(map[string]bool)
	seenImports := make(map[string]bool)
	v.writeImportedAssets(&b, chunk, seenCSS, seenImports)
	v.writeCSS(&b, chunk.CSS, seenCSS)
	fmt.Fprintf(&b, "<script type=\"module\" src=\"/public/dist/%s\"></script>\n", template.HTMLEscapeString(chunk.File))

	return template.HTML(b.String())
}

func (v viteAssets) writeImportedAssets(b *strings.Builder, chunk viteChunk, seenCSS, seenImports map[string]bool) {
	for _, importKey := range chunk.Imports {
		if seenImports[importKey] {
			continue
		}
		seenImports[importKey] = true

		imported, ok := v.manifest[importKey]
		if !ok {
			continue
		}
		v.writeImportedAssets(b, imported, seenCSS, seenImports)
		v.writeCSS(b, imported.CSS, seenCSS)
		fmt.Fprintf(b, "<link rel=\"modulepreload\" href=\"/public/dist/%s\">\n", template.HTMLEscapeString(imported.File))
	}
}

func (v viteAssets) writeCSS(b *strings.Builder, files []string, seen map[string]bool) {
	for _, file := range files {
		if seen[file] {
			continue
		}
		seen[file] = true
		fmt.Fprintf(b, "<link rel=\"stylesheet\" href=\"/public/dist/%s\">\n", template.HTMLEscapeString(file))
	}
}

func jsonForTemplate(v interface{}) template.JS {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("Failed to marshal template JSON: %v", err)
		return "null"
	}
	return template.JS(data)
}
