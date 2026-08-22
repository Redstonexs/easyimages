package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"easyimage/config"
	"golang.org/x/crypto/hkdf"
)

const (
	CaptchaTypeBuiltin   = 0
	CaptchaTypeTurnstile = 1
	CaptchaTypeRecaptcha = 2
	CaptchaTypeCap       = 3
)

// captchaHTTPClient is used for every server-to-server captcha verification.
// A bounded timeout matters here: these calls sit directly in the login request
// path, so an unresponsive verifier must fail rather than pin the handler.
var captchaHTTPClient = &http.Client{Timeout: 10 * time.Second}

const (
	captchaTokenExpiry = 5 * time.Minute
	captchaHMACKey     = "easyimage-captcha-hmac"
)

// CaptchaData captcha generation response
type CaptchaData struct {
	Type     string `json:"type"`
	Question string `json:"question,omitempty"`
	Token    string `json:"token,omitempty"`
	SiteKey  string `json:"site_key,omitempty"`
	// APIEndpoint is the Cap widget endpoint (instance URL + site key, trailing slash).
	APIEndpoint string `json:"api_endpoint,omitempty"`
	// WidgetURL is the <script> source for the Cap widget.
	WidgetURL string `json:"widget_url,omitempty"`
	// WasmURL overrides where the widget fetches its proof-of-work WASM from.
	// Empty means "use the widget's own default", which is a public CDN.
	WasmURL string `json:"wasm_url,omitempty"`
}

// capAPIEndpoint builds the widget/verify base URL for a Cap Standalone instance.
// The trailing slash is mandatory — the widget appends "challenge"/"redeem" to it
// and the server appends "siteverify" — so it is normalized here rather than
// relying on the operator typing it correctly in the admin form.
func capAPIEndpoint(cfg *config.Config) string {
	return strings.TrimRight(cfg.CapInstanceURL, "/") + "/" + cfg.CapSiteKey + "/"
}

// capAssetURLs resolves where the browser loads the Cap widget and its WASM from.
//
// By default both come from the operator's own Cap instance, which serves them
// under /assets/ when the container has ENABLE_ASSETS_SERVER=true. That keeps
// visitors talking only to infrastructure the operator controls — the whole
// reason to pick a self-hosted captcha. Note the widget's WASM is a separate
// fetch that also defaults to a public CDN, so it has to be redirected too.
//
// Operators who cannot enable the asset server can point CapWidgetURL at a CDN
// build or a copy served from /public/static/; in that case the widget keeps its
// own default WASM source, since a CDN build expects the matching CDN WASM.
func capAssetURLs(cfg *config.Config) (widgetURL, wasmURL string) {
	if custom := strings.TrimSpace(cfg.CapWidgetURL); custom != "" {
		return custom, ""
	}
	base := strings.TrimRight(cfg.CapInstanceURL, "/")
	return base + "/assets/widget.js", base + "/assets/cap_wasm_bg.wasm"
}

// capConfigured reports whether Cap has everything it needs to run.
func capConfigured(cfg *config.Config) bool {
	return cfg.CapInstanceURL != "" && cfg.CapSiteKey != "" && cfg.CapSecretKey != ""
}

// generateCaptchaHMACKey derives a key from config password using HKDF-SHA256.
// HKDF (RFC 5869) is a proper KDF designed for key derivation from a shared secret.
func generateCaptchaHMACKey() []byte {
	secret := ""
	// config.Get() returns nil until the config has been loaded. Fall back to the
	// constant rather than panicking — the empty-password path already does.
	if cfg := config.Get(); cfg != nil {
		secret = cfg.Password
	}
	if secret == "" {
		secret = captchaHMACKey
	}
	// HKDF(secret, salt, info) → key material
	// salt = captchaHMACKey constant, info = "captcha-hmac-key"
	hkdfReader := hkdf.New(sha256.New, []byte(secret), []byte(captchaHMACKey), []byte("captcha-hmac-key"))
	key := make([]byte, 32) // SHA-256 output size
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		// Should never happen with a 32-byte request from SHA-256 HKDF
		log.Printf("[Captcha] HKDF key derivation failed: %v", err)
	}
	return key
}

// captchaTokenData data embedded in the captcha token
type captchaTokenData struct {
	Answer    string `json:"a"`
	ExpiresAt int64  `json:"e"`
}

// GenerateCaptcha creates a new captcha challenge based on config
func GenerateCaptcha(cfg *config.Config) CaptchaData {
	if cfg.Captcha == 0 {
		return CaptchaData{Type: "disabled"}
	}

	switch cfg.CaptchaType {
	case CaptchaTypeTurnstile:
		if cfg.TurnstileSiteKey == "" || cfg.TurnstileSecretKey == "" {
			// 密钥未配置，回退到内置验证码
			return generateBuiltinCaptcha()
		}
		return CaptchaData{
			Type:    "turnstile",
			SiteKey: cfg.TurnstileSiteKey,
		}
	case CaptchaTypeRecaptcha:
		if cfg.RecaptchaSiteKey == "" || cfg.RecaptchaSecretKey == "" {
			// 密钥未配置，回退到内置验证码
			return generateBuiltinCaptcha()
		}
		return CaptchaData{
			Type:    "recaptcha",
			SiteKey: cfg.RecaptchaSiteKey,
		}
	case CaptchaTypeCap:
		if !capConfigured(cfg) {
			// 实例地址或密钥未配置，回退到内置验证码
			return generateBuiltinCaptcha()
		}
		widgetURL, wasmURL := capAssetURLs(cfg)
		return CaptchaData{
			Type:        "cap",
			SiteKey:     cfg.CapSiteKey,
			APIEndpoint: capAPIEndpoint(cfg),
			WidgetURL:   widgetURL,
			WasmURL:     wasmURL,
		}
	default:
		return generateBuiltinCaptcha()
	}
}

// generateBuiltinCaptcha generates a math-based captcha
func generateBuiltinCaptcha() CaptchaData {
	a, _ := rand.Int(rand.Reader, big.NewInt(20))
	b, _ := rand.Int(rand.Reader, big.NewInt(20))
	answer := int(a.Int64()) + int(b.Int64())
	question := fmt.Sprintf("%d + %d = ?", a.Int64(), b.Int64())

	token, err := signCaptchaAnswer(answer)
	if err != nil {
		log.Printf("[Captcha] failed to sign answer: %v", err)
		return CaptchaData{Type: "disabled"}
	}

	return CaptchaData{
		Type:     "builtin",
		Question: question,
		Token:    token,
	}
}

// signCaptchaAnswer signs the captcha answer with HMAC-SHA256
func signCaptchaAnswer(answer int) (string, error) {
	data := captchaTokenData{
		Answer:    fmt.Sprintf("%d", answer),
		ExpiresAt: time.Now().Add(captchaTokenExpiry).Unix(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal token data: %w", err)
	}

	encoded := base64.URLEncoding.EncodeToString(jsonData)
	key := generateCaptchaHMACKey()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(encoded))
	sig := mac.Sum(nil)

	return encoded + "." + base64.URLEncoding.EncodeToString(sig), nil
}

// VerifyCaptcha verifies the captcha response
func VerifyCaptcha(cfg *config.Config, captchaAnswer, captchaToken, turnstileResponse, recaptchaResponse, capResponse string) (bool, string) {
	if cfg.Captcha == 0 {
		return true, ""
	}

	switch cfg.CaptchaType {
	case CaptchaTypeTurnstile:
		if cfg.TurnstileSecretKey == "" {
			return verifyBuiltinCaptcha(captchaAnswer, captchaToken)
		}
		return verifyTurnstile(cfg.TurnstileSecretKey, turnstileResponse)
	case CaptchaTypeRecaptcha:
		if cfg.RecaptchaSecretKey == "" {
			return verifyBuiltinCaptcha(captchaAnswer, captchaToken)
		}
		return verifyRecaptcha(cfg.RecaptchaSecretKey, recaptchaResponse)
	case CaptchaTypeCap:
		if !capConfigured(cfg) {
			return verifyBuiltinCaptcha(captchaAnswer, captchaToken)
		}
		return verifyCap(cfg, capResponse)
	default:
		return verifyBuiltinCaptcha(captchaAnswer, captchaToken)
	}
}

// verifyBuiltinCaptcha verifies the built-in math captcha
func verifyBuiltinCaptcha(answer, token string) (bool, string) {
	if answer == "" || token == "" {
		return false, "请输入验证码答案"
	}

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false, "验证码无效"
	}

	encoded := parts[0]
	sigBase64 := parts[1]

	sig, err := base64.URLEncoding.DecodeString(sigBase64)
	if err != nil {
		return false, "验证码无效"
	}

	key := generateCaptchaHMACKey()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(encoded))
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(sig, expectedSig) {
		return false, "验证码无效"
	}

	jsonData, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return false, "验证码无效"
	}

	var data captchaTokenData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return false, "验证码无效"
	}

	if time.Now().Unix() > data.ExpiresAt {
		return false, "验证码已过期，请刷新页面重新获取"
	}

	if strings.TrimSpace(answer) != data.Answer {
		return false, "验证码答案错误"
	}

	return true, ""
}

// verifyTurnstile verifies Cloudflare Turnstile response
func verifyTurnstile(secretKey, token string) (bool, string) {
	if secretKey == "" {
		return false, "Turnstile 未配置密钥"
	}
	if token == "" {
		return false, "请完成人机验证"
	}

	form := url.Values{}
	form.Set("secret", secretKey)
	form.Set("response", token)

	resp, err := captchaHTTPClient.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", form)
	if err != nil {
		log.Printf("[Captcha] Turnstile verification request failed: %v", err)
		return false, "人机验证服务请求失败"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Captcha] Failed to read Turnstile response: %v", err)
		return false, "人机验证服务响应读取失败"
	}

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[Captcha] Failed to parse Turnstile response: %v", err)
		return false, "人机验证服务响应解析失败"
	}

	if !result.Success {
		return false, "人机验证失败，请重试"
	}

	return true, ""
}

// verifyRecaptcha verifies Google reCAPTCHA response
func verifyRecaptcha(secretKey, token string) (bool, string) {
	if secretKey == "" {
		return false, "reCAPTCHA 未配置密钥"
	}
	if token == "" {
		return false, "请完成人机验证"
	}

	form := url.Values{}
	form.Set("secret", secretKey)
	form.Set("response", token)

	resp, err := captchaHTTPClient.PostForm("https://www.google.com/recaptcha/api/siteverify", form)
	if err != nil {
		log.Printf("[Captcha] reCAPTCHA verification request failed: %v", err)
		return false, "人机验证服务请求失败"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Captcha] Failed to read reCAPTCHA response: %v", err)
		return false, "人机验证服务响应读取失败"
	}

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[Captcha] Failed to parse reCAPTCHA response: %v", err)
		return false, "人机验证服务响应解析失败"
	}

	if !result.Success {
		return false, "人机验证失败，请重试"
	}

	return true, ""
}

// verifyCap verifies a Cap token against a self-hosted Cap Standalone instance.
//
// The contract is deliberately reCAPTCHA-shaped: POST {secret, response} to
// <instance>/<siteKey>/siteverify and read back {success}. Cap tokens are
// single-use — the instance deletes the token on verification — so this must be
// called exactly once per login attempt, before any side effect.
//
// Every failure path returns false (fail closed). A verifier that cannot be
// reached must reject the login, never wave it through.
func verifyCap(cfg *config.Config, token string) (bool, string) {
	if !capConfigured(cfg) {
		return false, "Cap 未配置实例地址或密钥"
	}
	if token == "" {
		return false, "请完成人机验证"
	}

	payload, err := json.Marshal(map[string]string{
		"secret":   cfg.CapSecretKey,
		"response": token,
	})
	if err != nil {
		log.Printf("[Captcha] Failed to encode Cap request: %v", err)
		return false, "人机验证服务请求失败"
	}

	endpoint := capAPIEndpoint(cfg) + "siteverify"
	resp, err := captchaHTTPClient.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[Captcha] Cap verification request failed: %v", err)
		return false, "人机验证服务请求失败"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Printf("[Captcha] Failed to read Cap response: %v", err)
		return false, "人机验证服务响应读取失败"
	}

	if resp.StatusCode != http.StatusOK {
		// A 401 here almost always means the admin key was pasted where the
		// secret key (sk-...) belongs.
		log.Printf("[Captcha] Cap verification returned HTTP %d", resp.StatusCode)
		return false, "人机验证服务响应异常"
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[Captcha] Failed to parse Cap response: %v", err)
		return false, "人机验证服务响应解析失败"
	}

	if !result.Success {
		if result.Error != "" {
			log.Printf("[Captcha] Cap rejected token: %s", result.Error)
		}
		return false, "人机验证失败，请重试"
	}

	return true, ""
}
