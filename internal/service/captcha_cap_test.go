package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easyimage/config"
)

func capConfig(instanceURL string) *config.Config {
	return &config.Config{
		Captcha:        1,
		CaptchaType:    CaptchaTypeCap,
		CapInstanceURL: instanceURL,
		CapSiteKey:     "a1b2c3d4e5",
		CapSecretKey:   "sk-test-secret",
	}
}

func TestCapAPIEndpointNormalizesTrailingSlash(t *testing.T) {
	// A missing or doubled trailing slash is the most common Cap setup mistake,
	// so the operator's input is normalized rather than trusted.
	for _, instance := range []string{
		"https://cap.example.com",
		"https://cap.example.com/",
		"https://cap.example.com///",
	} {
		got := capAPIEndpoint(capConfig(instance))
		if want := "https://cap.example.com/a1b2c3d4e5/"; got != want {
			t.Errorf("capAPIEndpoint(%q) = %q, want %q", instance, got, want)
		}
	}
}

func TestCapAssetURLsDefaultToTheInstance(t *testing.T) {
	widget, wasm := capAssetURLs(capConfig("https://cap.example.com/"))
	if want := "https://cap.example.com/assets/widget.js"; widget != want {
		t.Errorf("widget URL = %q, want %q", widget, want)
	}
	// The WASM is a separate fetch that otherwise defaults to a public CDN.
	// Leaving it unset would leak visitors to a third party.
	if want := "https://cap.example.com/assets/cap_wasm_bg.wasm"; wasm != want {
		t.Errorf("wasm URL = %q, want %q", wasm, want)
	}
}

func TestCapAssetURLsHonorOverride(t *testing.T) {
	cfg := capConfig("https://cap.example.com")
	cfg.CapWidgetURL = "  /public/static/cap/widget.js  "
	widget, wasm := capAssetURLs(cfg)
	if want := "/public/static/cap/widget.js"; widget != want {
		t.Errorf("widget URL = %q, want %q", widget, want)
	}
	if wasm != "" {
		t.Errorf("wasm URL = %q, want empty so the widget uses its matching default", wasm)
	}
}

func TestGenerateCaptchaCapFallsBackWhenIncomplete(t *testing.T) {
	for name, mutate := range map[string]func(*config.Config){
		"no instance": func(c *config.Config) { c.CapInstanceURL = "" },
		"no site key": func(c *config.Config) { c.CapSiteKey = "" },
		"no secret":   func(c *config.Config) { c.CapSecretKey = "" },
	} {
		cfg := capConfig("https://cap.example.com")
		mutate(cfg)
		if got := GenerateCaptcha(cfg).Type; got != "builtin" {
			t.Errorf("%s: GenerateCaptcha type = %q, want builtin fallback", name, got)
		}
	}
}

func TestGenerateCaptchaCapWhenConfigured(t *testing.T) {
	data := GenerateCaptcha(capConfig("https://cap.example.com"))
	if data.Type != "cap" {
		t.Fatalf("type = %q, want cap", data.Type)
	}
	if data.APIEndpoint != "https://cap.example.com/a1b2c3d4e5/" {
		t.Errorf("api endpoint = %q", data.APIEndpoint)
	}
	// The site key is public, but the secret key must never reach the browser.
	blob, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "sk-test-secret") {
		t.Fatalf("secret key leaked into the client payload: %s", blob)
	}
}

// capVerifyServer stands in for a Cap Standalone instance and records the one
// request it receives.
func capVerifyServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var gotReq http.Request
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = *r
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &gotReq, &gotBody
}

func TestVerifyCapSuccess(t *testing.T) {
	server, gotReq, gotBody := capVerifyServer(t, http.StatusOK, `{"success":true}`)

	ok, msg := verifyCap(capConfig(server.URL), "cap-token-value")
	if !ok {
		t.Fatalf("verifyCap failed: %s", msg)
	}

	if gotReq.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", gotReq.Method)
	}
	if want := "/a1b2c3d4e5/siteverify"; gotReq.URL.Path != want {
		t.Errorf("path = %q, want %q", gotReq.URL.Path, want)
	}
	if ct := gotReq.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q, want application/json", ct)
	}
	var payload map[string]string
	if err := json.Unmarshal(*gotBody, &payload); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, *gotBody)
	}
	if payload["secret"] != "sk-test-secret" || payload["response"] != "cap-token-value" {
		t.Errorf("payload = %v", payload)
	}
}

func TestVerifyCapRejectsMissingToken(t *testing.T) {
	// An attacker posting the form without the widget sends no token at all.
	// This must be rejected without even asking the instance.
	server, gotReq, _ := capVerifyServer(t, http.StatusOK, `{"success":true}`)
	if ok, _ := verifyCap(capConfig(server.URL), ""); ok {
		t.Fatal("empty token was accepted")
	}
	if gotReq.Method != "" {
		t.Error("an empty token should not reach the Cap instance")
	}
}

func TestVerifyCapFailsClosed(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"rejected token":  {http.StatusOK, `{"success":false,"error":"invalid token"}`},
		"bad credentials": {http.StatusUnauthorized, `{"success":false}`},
		"instance error":  {http.StatusInternalServerError, `boom`},
		"garbage body":    {http.StatusOK, `not json`},
	}
	for name, tc := range cases {
		server, _, _ := capVerifyServer(t, tc.status, tc.body)
		ok, msg := verifyCap(capConfig(server.URL), "cap-token-value")
		if ok {
			t.Errorf("%s: verification passed, want rejection", name)
		}
		if msg == "" {
			t.Errorf("%s: rejection carried no message", name)
		}
	}
}

func TestVerifyCapUnreachableInstanceIsRejected(t *testing.T) {
	// Closing immediately gives us an address nothing is listening on.
	server, _, _ := capVerifyServer(t, http.StatusOK, `{"success":true}`)
	url := server.URL
	server.Close()

	if ok, _ := verifyCap(capConfig(url), "cap-token-value"); ok {
		t.Fatal("an unreachable Cap instance must not grant access")
	}
}

func TestVerifyCaptchaRoutesCapToken(t *testing.T) {
	server, gotBody, _ := capVerifyServer(t, http.StatusOK, `{"success":true}`)
	cfg := capConfig(server.URL)

	// The cap token is the 5th response argument; the others must be ignored.
	ok, msg := VerifyCaptcha(cfg, "", "", "turnstile-junk", "recaptcha-junk", "cap-token-value")
	if !ok {
		t.Fatalf("VerifyCaptcha rejected a valid Cap token: %s", msg)
	}
	if gotBody.Method != http.MethodPost {
		t.Error("VerifyCaptcha did not reach the Cap instance")
	}
}

func TestVerifyCaptchaCapWithoutSecretFallsBackToBuiltin(t *testing.T) {
	cfg := capConfig("https://cap.example.com")
	cfg.CapSecretKey = ""

	// Falling back means the builtin verifier runs — which rejects an empty
	// answer rather than letting the request through.
	if ok, _ := VerifyCaptcha(cfg, "", "", "", "", "cap-token-value"); ok {
		t.Fatal("misconfigured Cap must not grant access")
	}
}
