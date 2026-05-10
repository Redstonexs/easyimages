package service

import (
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
)

const (
	CaptchaTypeBuiltin   = 0
	CaptchaTypeTurnstile = 1
	CaptchaTypeRecaptcha = 2
)

const (
	captchaTokenExpiry = 5 * time.Minute
	captchaHMACKey     = "easyimage-captcha-hmac"
)

// CaptchaData captcha generation response
type CaptchaData struct {
	Type    string `json:"type"`
	Question string `json:"question,omitempty"`
	Token   string `json:"token,omitempty"`
	SiteKey string `json:"site_key,omitempty"`
}

// generateCaptchaHMACKey derives a key from config password for HMAC signing
func generateCaptchaHMACKey() []byte {
	cfg := config.Get()
	secret := cfg.Password
	if secret == "" {
		secret = captchaHMACKey
	}
	h := sha256.Sum256([]byte(captchaHMACKey + secret))
	return h[:]
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
func VerifyCaptcha(cfg *config.Config, captchaAnswer, captchaToken, turnstileResponse, recaptchaResponse string) (bool, string) {
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

	resp, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", form)
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

	resp, err := http.PostForm("https://www.google.com/recaptcha/api/siteverify", form)
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
