package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckAndMigrateNoOpWithoutPHPFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}

	if err := checkAndMigrate(); err != nil {
		t.Fatalf("checkAndMigrate() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join("config", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("config.json should not be created when no PHP files exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join("config", "install.lock")); !os.IsNotExist(err) {
		t.Fatalf("install.lock should not be created when no PHP files exist: %v", err)
	}
}

func TestCheckAndMigrateConvertsAndDeletesLegacyPHPFiles(t *testing.T) {
	t.Chdir(t.TempDir())
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
	'maxSize'=>5242880,
	'update'=>'2026-06-02 00:00:00'
	);
`
	apiKeyPHP := `<?php
$tokenList=Array
	(
	'1c17b11693cb5ec63859b091c5b9c1b2'=>Array
		(
		'id'=>1,
		'expired'=>2544411528,
		'add_time'=>1680497928
		)
	);
`
	guestPHP := `<?php
$guestConfig=Array
	(
	'guest'=>Array
		(
		'password'=>'84983c60f7daadc1cb8698621f802c0d9f9a3c3c295c810748fb048115c186ec',
		'expired'=>1765301956,
		'add_time'=>1678988356
		)
	);
`
	for name, content := range map[string]string{
		"config.php":       realPHPConfig,
		"api_key.php":      apiKeyPHP,
		"config.guest.php": guestPHP,
	} {
		if err := os.WriteFile(filepath.Join("config", name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
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
	apiData, err := os.ReadFile(filepath.Join("config", "api_key.json"))
	if err != nil {
		t.Fatalf("read migrated api_key.json: %v", err)
	}
	if !strings.Contains(string(apiData), "1c17b11693cb5ec63859b091c5b9c1b2") {
		t.Fatalf("api_key.json missing migrated token: %s", apiData)
	}
	if _, err := os.Stat(filepath.Join("config", "install.lock")); err != nil {
		t.Fatalf("expected install.lock after migration: %v", err)
	}
	for _, removed := range []string{"config.php", "api_key.php", "config.guest.php", "php_backup"} {
		if _, err := os.Stat(filepath.Join("config", removed)); !os.IsNotExist(err) {
			t.Fatalf("expected config/%s to be absent after migration, stat err=%v", removed, err)
		}
	}
}
