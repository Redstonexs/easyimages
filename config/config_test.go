package config

import "testing"

func TestSetDefaultsSetsSiteIcon(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	if cfg.SiteIcon != "/favicon.ico" {
		t.Fatalf("SiteIcon = %q, want /favicon.ico", cfg.SiteIcon)
	}
}
