package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"easyimage/internal/service"
)

const maxQuotaBytes = int64(1 << 50)

type Identity struct {
	ID          string
	AuthVersion uint64
}

func (actor Identity) IsAdmin() bool { return actor.ID == AdminID }

type account struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	TokenHash    string    `json:"token_hash,omitempty"`
	QuotaBytes   int64     `json:"quota_bytes"`
	Disabled     bool      `json:"disabled"`
	AuthVersion  uint64    `json:"auth_version"`
	CreatedAt    time.Time `json:"created_at"`
}

type User struct {
	ID         string     `json:"id"`
	Username   string     `json:"username"`
	Role       string     `json:"role"`
	QuotaBytes int64      `json:"quota_bytes"`
	UsedBytes  int64      `json:"used_bytes"`
	Disabled   bool       `json:"disabled"`
	HasToken   bool       `json:"has_token"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
}

type CreateUser struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	QuotaBytes int64  `json:"quota_bytes"`
}

type UpdateUser struct {
	Password   *string `json:"password"`
	QuotaBytes *int64  `json:"quota_bytes"`
	Disabled   *bool   `json:"disabled"`
}

type QuotaError struct {
	QuotaBytes     int64 `json:"quota_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
	RequestedBytes int64 `json:"requested_bytes,omitempty"`
}

func (e *QuotaError) Error() string {
	return "the original or variant would exceed the account's storage quota"
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)

func (s *Store) CreateUser(actor Identity, adminUsername string, input CreateUser) (User, error) {
	if !actor.IsAdmin() {
		return User{}, ErrForbidden
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	if !usernamePattern.MatchString(input.Username) {
		return User{}, ValidationError("username must contain 3–32 letters, digits, dots, dashes or underscores, starting with a letter or digit")
	}
	if err := validateQuota(input.QuotaBytes); err != nil {
		return User{}, err
	}
	hash, err := hashNewPassword(input.Password)
	if err != nil {
		return User{}, err
	}
	id, err := randomHex(16)
	if err != nil {
		return User{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.EqualFold(input.Username, adminUsername) || s.accountByUsernameLocked(input.Username).ID != "" || s.catalog.Accounts[id].ID != "" {
		return User{}, ErrConflict
	}
	user := account{
		ID: id, Username: input.Username, PasswordHash: hash,
		QuotaBytes: input.QuotaBytes, AuthVersion: 1, CreatedAt: time.Now().UTC(),
	}
	next := s.catalog
	next.Accounts = maps.Clone(next.Accounts)
	next.Accounts[id] = user
	if err := s.persistCatalogLocked(next); err != nil {
		return User{}, err
	}
	return s.userViewLocked(user), nil
}

func (s *Store) UpdateUser(actor Identity, id string, input UpdateUser) (User, error) {
	if !actor.IsAdmin() {
		return User{}, ErrForbidden
	}
	if input.QuotaBytes != nil {
		if err := validateQuota(*input.QuotaBytes); err != nil {
			return User{}, err
		}
	}
	var hash string
	if input.Password != nil {
		var err error
		hash, err = hashNewPassword(*input.Password)
		if err != nil {
			return User{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.catalog.Accounts[id]
	if !ok {
		return User{}, ErrNotFound
	}
	if input.QuotaBytes != nil {
		user.QuotaBytes = *input.QuotaBytes
	}
	if hash != "" || (input.Disabled != nil && *input.Disabled != user.Disabled) {
		user.AuthVersion++
		user.TokenHash = ""
	}
	if input.Disabled != nil {
		user.Disabled = *input.Disabled
	}
	if hash != "" {
		user.PasswordHash = hash
	}
	next := s.catalog
	next.Accounts = maps.Clone(next.Accounts)
	next.Accounts[id] = user
	if err := s.persistCatalogLocked(next); err != nil {
		return User{}, err
	}
	s.rebuildTokenIndexLocked()
	return s.userViewLocked(user), nil
}

func (s *Store) Users(actor Identity) ([]User, error) {
	if !actor.IsAdmin() {
		return nil, ErrForbidden
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]User, 0, len(s.catalog.Accounts))
	for _, user := range s.catalog.Accounts {
		users = append(users, s.userViewLocked(user))
	}
	slices.SortFunc(users, func(a, b User) int { return strings.Compare(a.Username, b.Username) })
	return users, nil
}

func (s *Store) Me(actor Identity, adminUsername string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.authorizedLocked(actor) {
		return User{}, ErrUnauthorized
	}
	if actor.IsAdmin() {
		return User{ID: AdminID, Username: adminUsername, Role: "admin", UsedBytes: s.usage[AdminID], HasToken: s.catalog.AdminTokenHash != ""}, nil
	}
	return s.userViewLocked(s.catalog.Accounts[actor.ID]), nil
}

func (s *Store) userViewLocked(user account) User {
	return User{
		ID: user.ID, Username: user.Username, Role: "user", QuotaBytes: user.QuotaBytes,
		UsedBytes: s.usage[user.ID], Disabled: user.Disabled, HasToken: user.TokenHash != "", CreatedAt: &user.CreatedAt,
	}
}

func (s *Store) accountByUsernameLocked(name string) account {
	for _, user := range s.catalog.Accounts {
		if user.Username == name {
			return user
		}
	}
	return account{}
}

func (s *Store) Login(username, password string) (Identity, bool) {
	// Compare a bcrypt hash for unknown usernames as well.
	const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	s.mu.RLock()
	user := s.accountByUsernameLocked(strings.ToLower(strings.TrimSpace(username)))
	s.mu.RUnlock()
	hash := user.PasswordHash
	if hash == "" {
		hash = dummyHash
	}
	if len(password) > 72 || !service.CheckPassword(password, hash) || user.ID == "" || user.Disabled {
		return Identity{}, false
	}
	actor := Identity{ID: user.ID, AuthVersion: user.AuthVersion}
	return actor, s.Authorized(actor)
}

func (s *Store) Authorized(actor Identity) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authorizedLocked(actor)
}

func (s *Store) authorizedLocked(actor Identity) bool {
	if actor.IsAdmin() {
		return true
	}
	user, ok := s.catalog.Accounts[actor.ID]
	return ok && !user.Disabled && user.AuthVersion == actor.AuthVersion
}

func (s *Store) ownerLocked(actor Identity, ownerID string) (string, error) {
	if !s.authorizedLocked(actor) {
		return "", ErrUnauthorized
	}
	if ownerID == "" || ownerID == actor.ID {
		return actor.ID, nil
	}
	if !actor.IsAdmin() {
		return "", ErrForbidden
	}
	if _, ok := s.catalog.Accounts[ownerID]; !ok {
		return "", ErrNotFound
	}
	return ownerID, nil
}

func (s *Store) AuthenticateToken(token string) (Identity, bool) {
	if !strings.HasPrefix(token, "ei_") || len(token) != 67 {
		return Identity{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.tokenOwner[tokenDigest(token)]
	return Identity{ID: id, AuthVersion: s.catalog.Accounts[id].AuthVersion}, ok
}

func (s *Store) RotateToken(actor Identity) (string, error) {
	id, err := randomHex(32)
	if err != nil {
		return "", err
	}
	token := "ei_" + id
	if err := s.saveToken(actor, tokenDigest(token)); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) RevokeToken(actor Identity) error {
	return s.saveToken(actor, "")
}

func (s *Store) saveToken(actor Identity, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorizedLocked(actor) {
		return ErrUnauthorized
	}
	next := s.catalog
	if actor.IsAdmin() {
		next.AdminTokenHash = hash
	} else {
		next.Accounts = maps.Clone(next.Accounts)
		user := next.Accounts[actor.ID]
		user.TokenHash = hash
		next.Accounts[actor.ID] = user
	}
	if err := s.persistCatalogLocked(next); err != nil {
		return err
	}
	s.rebuildTokenIndexLocked()
	return nil
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func validateQuota(quota int64) error {
	if quota < 0 || quota > maxQuotaBytes {
		return ValidationError(fmt.Sprintf("quota_bytes must be between 0 and %d", maxQuotaBytes))
	}
	return nil
}

func hashNewPassword(password string) (string, error) {
	if len([]rune(password)) < 8 || len(password) > 72 {
		return "", ValidationError("password must contain at least 8 characters and be at most 72 UTF-8 bytes")
	}
	if hash := service.HashPassword(password); hash != "" {
		return hash, nil
	}
	return "", fmt.Errorf("could not hash user password")
}
