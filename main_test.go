package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckAndMigrateSkipsBundledDefaultPHPConfig(t *testing.T) {
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

	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	phpConfig := `<?php
$config=Array
	(
	'title'=>'简单图床 - EasyImage',
	'domain'=>'http://127.0.0.1',
	'imgurl'=>'http://127.0.0.1',
	'user'=>'admin',
	'password'=>'7676aaafb027c825bd9abab78b234070e702752f625b752e55e55b48e607e358',
	'path'=>'/i/',
	'storage_path'=>'Y/m/d/',
	'update'=>'2025-07-04 19:28:57'
	);
`
	if err := os.WriteFile(filepath.Join("config", "config.php"), []byte(phpConfig), 0644); err != nil {
		t.Fatalf("write php config: %v", err)
	}

	if err := checkAndMigrate(); err != nil {
		t.Fatalf("checkAndMigrate() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join("config", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("config.json exists after default PHP config migration check: %v", err)
	}
	if _, err := os.Stat(filepath.Join("config", "install.lock")); !os.IsNotExist(err) {
		t.Fatalf("install.lock exists after default PHP config migration check: %v", err)
	}
}

func TestCheckAndMigrateOverwritesDefaultGoConfigForRealPHPConfig(t *testing.T) {
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

	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	defaultGoConfig := `{"domain":"http://127.0.0.1:8080","password":"7676aaafb027c825bd9abab78b234070e702752f625b752e55e55b48e607e358"}`
	if err := os.WriteFile(filepath.Join("config", "config.json"), []byte(defaultGoConfig), 0644); err != nil {
		t.Fatalf("write default go config: %v", err)
	}
	realPHPConfig := `<?php
$config=Array
	(
	'title'=>'Migrated EasyImage',
	'domain'=>'https://img.example.com',
	'imgurl'=>'https://img.example.com',
	'user'=>'admin',
	'password'=>'7676aaafb027c825bd9abab78b234070e702752f625b752e55e55b48e607e358',
	'path'=>'/i/',
	'storage_path'=>'Y/m/d/',
	'update'=>'2026-06-02 00:00:00'
	);
`
	if err := os.WriteFile(filepath.Join("config", "config.php"), []byte(realPHPConfig), 0644); err != nil {
		t.Fatalf("write php config: %v", err)
	}

	if err := checkAndMigrate(); err != nil {
		t.Fatalf("checkAndMigrate() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join("config", "config.json"))
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if !strings.Contains(string(data), "https://img.example.com") {
		t.Fatalf("config.json was not overwritten with migrated domain: %s", data)
	}
	if _, err := os.Stat(filepath.Join("config", "install.lock")); err != nil {
		t.Fatalf("expected install.lock after migration: %v", err)
	}
}
