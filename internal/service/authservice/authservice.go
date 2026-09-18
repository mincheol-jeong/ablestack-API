package authservice

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/service/licenseservice"
	"github.com/openwall/yescrypt-go"
	crypt "github.com/sergeymakinen/go-crypt"
	_ "github.com/sergeymakinen/go-crypt/bcrypt"
	_ "github.com/sergeymakinen/go-crypt/md5"
	_ "github.com/sergeymakinen/go-crypt/sha256"
	_ "github.com/sergeymakinen/go-crypt/sha512"
)

const (
	DefaultAccessTokenTTL = time.Hour
	TokenTypeBearer       = "Bearer"
	tokenUseAccess        = "access"
	randomTokenBytes      = 32
)

type Config struct {
	Linux                 LinuxConfig `json:"linux"`
	AccessTokenTTLSeconds int64       `json:"access_token_ttl_seconds"`
}

type LinuxConfig struct {
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups"`
}

type TokenClaim struct {
	Subject string `json:"sub"`
	Role    string `json:"role"`
	Use     string `json:"use"`
	Issued  int64  `json:"iat"`
	Expires int64  `json:"exp"`
}

type IssuedToken struct {
	TokenType     string `json:"token_type"`
	AccessToken   string `json:"access_token"`
	Authorization string `json:"authorization"`
	ExpiresIn     int64  `json:"expires_in"`
	Subject       string `json:"subject,omitempty"`
}

type tokenHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

func IssueAccessToken(subject string) (IssuedToken, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return IssuedToken{}, fmt.Errorf("subject required")
	}
	secret, err := SigningSecret()
	if err != nil {
		return IssuedToken{}, err
	}
	ttl := AccessTokenTTL()
	now := time.Now()
	claim := TokenClaim{
		Subject: subject,
		Role:    "admin",
		Use:     tokenUseAccess,
		Issued:  now.Unix(),
		Expires: now.Add(ttl).Unix(),
	}
	token, err := signToken(claim, secret)
	if err != nil {
		return IssuedToken{}, err
	}
	return IssuedToken{
		TokenType:     TokenTypeBearer,
		AccessToken:   token,
		Authorization: TokenTypeBearer + " " + token,
		ExpiresIn:     int64(ttl.Seconds()),
		Subject:       subject,
	}, nil
}

func IssueAccessTokenForLinuxUser(username string) (IssuedToken, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return IssuedToken{}, fmt.Errorf("linux user required")
	}
	if !IsSafeLinuxUsername(username) {
		return IssuedToken{}, fmt.Errorf("invalid linux user")
	}
	if !LinuxAccountAllowed(username, LoadConfig().Linux) {
		return IssuedToken{}, fmt.Errorf("linux user is not allowed")
	}
	return IssueAccessToken(username)
}

func ClaimsFromRequest(req *http.Request) (TokenClaim, error) {
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if authHeader == "" {
		return TokenClaim{}, fmt.Errorf("authorization header required")
	}
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], TokenTypeBearer) {
		return TokenClaim{}, fmt.Errorf("bearer token required")
	}
	secret, err := ExistingSigningSecret()
	if err != nil {
		return TokenClaim{}, err
	}
	return VerifyToken(parts[1], secret)
}

func VerifyLinuxCredentials(username string, password string) bool {
	username = strings.TrimSpace(username)
	if username == "" || password == "" || !IsSafeLinuxUsername(username) {
		return false
	}
	if !LinuxAccountAllowed(username, LoadConfig().Linux) {
		return false
	}
	return verifyLinuxShadowPassword(username, password)
}

func IsSafeLinuxUsername(username string) bool {
	if username == "" || len(username) > 128 {
		return false
	}
	for _, r := range username {
		if r == 0 || r == ':' || r == '/' || r == '\\' || r <= 31 || r == 127 {
			return false
		}
	}
	return true
}

func LinuxAccountAllowed(username string, cfg LinuxConfig) bool {
	if len(cfg.AllowedUsers) == 0 && len(cfg.AllowedGroups) == 0 {
		return true
	}
	for _, allowed := range cfg.AllowedUsers {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" || allowed == username {
			return true
		}
	}
	if len(cfg.AllowedGroups) == 0 {
		return false
	}
	groups := LinuxUserGroups(username)
	if len(groups) == 0 {
		return false
	}
	allowedGroups := map[string]struct{}{}
	for _, group := range cfg.AllowedGroups {
		group = strings.TrimSpace(group)
		if group != "" {
			allowedGroups[group] = struct{}{}
		}
	}
	for _, group := range groups {
		if _, ok := allowedGroups[group]; ok {
			return true
		}
	}
	return false
}

func LinuxUserGroups(username string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "id", "-nG", username).Output()
	if err != nil || ctx.Err() != nil {
		return nil
	}
	fields := strings.Fields(string(out))
	groups := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field) != "" {
			groups = append(groups, strings.TrimSpace(field))
		}
	}
	return groups
}

func CurrentLinuxUsername() (string, error) {
	if u, err := user.LookupId(strconv.Itoa(os.Geteuid())); err == nil && IsSafeLinuxUsername(u.Username) {
		return u.Username, nil
	}
	current, err := user.Current()
	if err != nil {
		return "", err
	}
	if !IsSafeLinuxUsername(current.Username) {
		return "", fmt.Errorf("invalid current user")
	}
	return current.Username, nil
}

func AuthRequired() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ABLESTACK_AUTH_REQUIRED"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return true
}

func AccessTokenTTL() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("ABLESTACK_ACCESS_TOKEN_TTL_SECONDS")); raw != "" {
		if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	cfg := LoadConfig()
	if cfg.AccessTokenTTLSeconds > 0 {
		return time.Duration(cfg.AccessTokenTTLSeconds) * time.Second
	}
	return DefaultAccessTokenTTL
}

func SigningSecret() (string, error) {
	return ExistingSigningSecret()
}

func ExistingSigningSecret() (string, error) {
	secret, err := licenseservice.CurrentAuthSecret()
	if err == nil {
		return secret, nil
	}
	return "", fmt.Errorf("active license required: %w", err)
}

func VerifyToken(token string, secret string) (TokenClaim, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return TokenClaim{}, fmt.Errorf("invalid token")
	}
	unsigned := parts[0] + "." + parts[1]
	expected := sign(unsigned, secret)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[2])) != 1 {
		return TokenClaim{}, fmt.Errorf("invalid token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return TokenClaim{}, fmt.Errorf("invalid token")
	}
	var claim TokenClaim
	if err := json.Unmarshal(payload, &claim); err != nil {
		return TokenClaim{}, fmt.Errorf("invalid token")
	}
	if claim.Use != tokenUseAccess || claim.Expires <= time.Now().Unix() {
		return TokenClaim{}, fmt.Errorf("token expired")
	}
	return claim, nil
}

func LoadConfig() Config {
	path := ConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}
	}
	cfg := Config{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return cfg
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}
	}
	return cfg
}

func SaveConfig(cfg Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(path, 0o600)
}

func ConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("ABLESTACK_AUTH_CONFIG")); path != "" {
		return path
	}
	base := strings.TrimSpace(os.Getenv("ABLESTACK_CONFIG_PATH"))
	if base == "" {
		base = "/etc/ablestack"
		if _, err := os.Stat(filepath.Join(base, "auth.json")); err != nil {
			if _, devErr := os.Stat(filepath.Join("configs", "auth.json")); devErr == nil {
				return filepath.Join("configs", "auth.json")
			}
		}
	}
	return filepath.Join(base, "auth.json")
}

func verifyLinuxShadowPassword(username string, password string) bool {
	hash, err := readLinuxShadowHash("/etc/shadow", username)
	if err != nil {
		return false
	}
	return verifyLinuxPasswordHash(hash, password)
}

func readLinuxShadowHash(path string, username string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), ":", 3)
		if len(fields) < 2 || fields[0] != username {
			continue
		}
		hash := strings.TrimSpace(fields[1])
		if hash == "" || strings.HasPrefix(hash, "!") || strings.HasPrefix(hash, "*") {
			return "", fmt.Errorf("linux account is locked")
		}
		return hash, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("linux account not found")
}

func verifyLinuxPasswordHash(hash string, password string) bool {
	if strings.HasPrefix(hash, "$y$") {
		candidate, err := yescrypt.Hash([]byte(password), []byte(hash))
		if err != nil {
			return false
		}
		return subtle.ConstantTimeCompare(candidate, []byte(hash)) == 1
	}
	if err := crypt.Check(hash, password); err != nil {
		return false
	}
	return true
}

func signToken(claim TokenClaim, secret string) (string, error) {
	header := tokenHeader{Alg: "HS256", Typ: "JWT"}
	headerRaw, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimRaw, err := json.Marshal(claim)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(headerRaw) + "." + base64.RawURLEncoding.EncodeToString(claimRaw)
	signature := sign(unsigned, secret)
	return unsigned + "." + signature, nil
}

func sign(unsigned string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func generateToken() (string, error) {
	raw := make([]byte, randomTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
