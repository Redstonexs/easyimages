package config

import "testing"

func TestSetDefaultsSetsSiteIcon(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	if cfg.SiteIcon != "/favicon.ico" {
		t.Fatalf("SiteIcon = %q, want /favicon.ico", cfg.SiteIcon)
	}
}

func TestSetDefaultsAddsLocalStorageSource(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	if cfg.DefaultStorageSource != "local" {
		t.Fatalf("DefaultStorageSource = %q", cfg.DefaultStorageSource)
	}
	source, ok := cfg.StorageSourceByID("")
	if !ok || source.ID != "local" || source.Type != "local" {
		t.Fatalf("default source = %+v, ok=%v", source, ok)
	}
}

func TestDefaultConfigEnablesPrivateUploadMode(t *testing.T) {
	cfg := getDefaultConfig()

	if cfg.MustLogin != 1 {
		t.Fatalf("MustLogin = %d, want 1", cfg.MustLogin)
	}
}

func TestSetDefaultsFallsBackWhenCapIsIncomplete(t *testing.T) {
	// Selecting Cap without a reachable instance must degrade to the builtin
	// captcha rather than leaving login unprotected.
	newCap := func() *Config {
		return &Config{
			Captcha:        1,
			CaptchaType:    3,
			CapInstanceURL: "https://cap.example.com",
			CapSiteKey:     "a1b2c3d4e5",
			CapSecretKey:   "sk-x",
		}
	}

	cases := map[string]func(*Config){
		"no instance": func(c *Config) { c.CapInstanceURL = "" },
		"no site key": func(c *Config) { c.CapSiteKey = "" },
		"no secret":   func(c *Config) { c.CapSecretKey = "" },
	}
	for name, drop := range cases {
		cfg := newCap()
		drop(cfg)
		setDefaults(cfg)
		if cfg.CaptchaType != 0 {
			t.Errorf("%s: captcha type = %d, want 0 (builtin fallback)", name, cfg.CaptchaType)
		}
	}

	complete := newCap()
	setDefaults(complete)
	if complete.CaptchaType != 3 {
		t.Errorf("fully configured Cap was downgraded to type %d", complete.CaptchaType)
	}
}
