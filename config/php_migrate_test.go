package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePHPArrayMultilineAndNested(t *testing.T) {
	src := `<?php
// Line comment mentioning array(
/* Block comment mentioning [ ] */
$tokenList=Array
	(
	'1c17b11693cb5ec63859b091c5b9c1b2'=>Array
		(
		'id'=>0,
		'expired'=>2544411528,
		'add_time'=>1680497928,
		),
	'833c5611862efcaa9955205ee96125e9'=>array(
		'id'=>2,
		'expired'=>1680584583,
		'add_time'=>1680498183
	),
	'escaped'=>'it\'s \\ok',
	);
`
	parsed, err := parsePHPArray(src)
	if err != nil {
		t.Fatalf("parsePHPArray() error = %v", err)
	}
	keys := convertPHPToAPIKeys(parsed)
	if len(keys) != 2 {
		t.Fatalf("migrated %d api keys, want 2: %+v", len(keys), keys)
	}
	first := keys["1c17b11693cb5ec63859b091c5b9c1b2"]
	if first == nil || first.ID != 0 || first.Expired != 2544411528 || first.AddTime != 1680497928 {
		t.Fatalf("unexpected first api key: %+v", first)
	}
	second := keys["833c5611862efcaa9955205ee96125e9"]
	if second == nil || second.ID != 2 || second.Expired != 1680584583 || second.AddTime != 1680498183 {
		t.Fatalf("unexpected second api key: %+v", second)
	}
	if got, _ := phpString(parsed, "escaped"); got != `it's \ok` {
		t.Fatalf("escaped = %q, want %q", got, `it's \ok`)
	}
}

func TestParsePHPArrayShortSyntaxAndIndexedList(t *testing.T) {
	src := `<?php
return [
	'title' => "Short Syntax",
	'maxSize' => 1048576,
	'mustLogin' => true,
	'public_list' => ['time', 'today'],
];
`
	parsed, err := parsePHPArray(src)
	if err != nil {
		t.Fatalf("parsePHPArray() error = %v", err)
	}
	cfg := convertPHPToGoConfig(parsed)
	if cfg.Title != "Short Syntax" || cfg.MaxSize != 1048576 || cfg.MustLogin != 1 {
		t.Fatalf("unexpected converted config: title=%q maxSize=%d mustLogin=%d", cfg.Title, cfg.MaxSize, cfg.MustLogin)
	}
}

func TestAutoMigratePreservesPHPFileOnParseError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatal(err)
	}
	phpPath := filepath.Join("config", "config.php")
	if err := os.WriteFile(phpPath, []byte("<?php $config = Array('broken' => 'unterminated"), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := AutoMigrate()
	if err == nil || migrated {
		t.Fatalf("AutoMigrate() = (%v, %v), want (false, error)", migrated, err)
	}
	if _, statErr := os.Stat(phpPath); statErr != nil {
		t.Fatalf("broken config.php must not be deleted on failure: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join("config", "config.json")); !os.IsNotExist(statErr) {
		t.Fatalf("config.json must not be created on parse failure")
	}
}

func TestAutoMigrateSkipsWhenGoConfigAlreadyInstalled(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("config", 0755); err != nil {
		t.Fatal(err)
	}
	installed := map[string]any{
		"title":    "Existing Go Site",
		"domain":   "https://go.example.com",
		"password": "$2a$10$existinghashvalue",
	}
	raw, _ := json.Marshal(installed)
	if err := os.WriteFile(filepath.Join("config", "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	phpPath := filepath.Join("config", "config.php")
	if err := os.WriteFile(phpPath, []byte("<?php $config = Array('domain' => 'https://old.example.com');"), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := AutoMigrate()
	if err != nil || migrated {
		t.Fatalf("AutoMigrate() = (%v, %v), want (false, nil)", migrated, err)
	}
	got, err := os.ReadFile(filepath.Join("config", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("installed config.json was modified: %s", got)
	}
	if _, err := os.Stat(phpPath); err != nil {
		t.Fatalf("unmigrated config.php should be left untouched when migration is skipped: %v", err)
	}
}
