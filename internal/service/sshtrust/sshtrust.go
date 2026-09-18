package sshtrust

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

const (
	DefaultIdentifier           = "cloud@ccvm"
	DefaultAuthorizedKeysPath   = "/root/.ssh/authorized_keys"
	DefaultManagementSSHDir     = "/var/cloudstack/management/.ssh"
	DefaultManagementPublicKey  = "id_rsa.pub"
	DefaultManagementPrivateKey = "id_rsa"
)

var authorizedKeysMu sync.Mutex

type Result struct {
	Present     bool
	Changed     bool
	Fingerprint string
}

func ManagementSSHDir() string {
	if value := strings.TrimSpace(os.Getenv("ABLESTACK_CLOUDSTACK_SSH_DIR")); value != "" {
		return value
	}
	return DefaultManagementSSHDir
}

func ManagementPublicKeyPath() string {
	return filepath.Join(ManagementSSHDir(), DefaultManagementPublicKey)
}

func ManagementPrivateKeyPath() string {
	return filepath.Join(ManagementSSHDir(), DefaultManagementPrivateKey)
}

func ReadManagementPublicKey() (string, string, error) {
	raw, err := os.ReadFile(ManagementPublicKeyPath())
	if err != nil {
		return "", "", fmt.Errorf("read CloudStack management public key: %w", err)
	}
	normalized, fingerprint, err := NormalizePublicKey(string(raw), DefaultIdentifier)
	if err != nil {
		return "", "", err
	}
	return normalized, fingerprint, nil
}

func NormalizePublicKey(raw string, identifier string) (string, string, error) {
	identifier = normalizeIdentifier(identifier)
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(raw)))
	if err != nil {
		return "", "", fmt.Errorf("invalid OpenSSH public key: %w", err)
	}
	normalized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + " " + identifier
	return normalized, ssh.FingerprintSHA256(key), nil
}

func EnsureAuthorizedKey(path string, rawKey string, identifier string) (Result, error) {
	identifier = normalizeIdentifier(identifier)
	normalized, fingerprint, err := NormalizePublicKey(rawKey, identifier)
	if err != nil {
		return Result{}, err
	}
	return updateAuthorizedKeys(path, identifier, normalized, fingerprint, false)
}

func RemoveAuthorizedKey(path string, identifier string) (Result, error) {
	return updateAuthorizedKeys(path, normalizeIdentifier(identifier), "", "", true)
}

func Status(path string, identifier string) (Result, error) {
	identifier = normalizeIdentifier(identifier)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, err
	}
	for _, line := range splitLines(raw) {
		key, comment, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(line))
		if parseErr == nil && strings.TrimSpace(comment) == identifier {
			return Result{Present: true, Fingerprint: ssh.FingerprintSHA256(key)}, nil
		}
	}
	return Result{}, nil
}

func updateAuthorizedKeys(path string, identifier string, normalized string, fingerprint string, remove bool) (Result, error) {
	authorizedKeysMu.Lock()
	defer authorizedKeysMu.Unlock()

	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	lines := splitLines(raw)
	out := make([]string, 0, len(lines)+1)
	unchanged := false
	for _, line := range lines {
		key, comment, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(line))
		if parseErr == nil && strings.TrimSpace(comment) == identifier {
			if !remove && strings.TrimSpace(line) == normalized && !unchanged {
				out = append(out, normalized)
				unchanged = true
				fingerprint = ssh.FingerprintSHA256(key)
			}
			continue
		}
		out = append(out, line)
	}
	if !remove && !unchanged {
		out = append(out, normalized)
	}

	changed := strings.Join(lines, "\n") != strings.Join(out, "\n")
	if changed {
		if err := writeAuthorizedKeysAtomic(path, out); err != nil {
			return Result{}, err
		}
	}
	return Result{Present: !remove, Changed: changed, Fingerprint: fingerprint}, nil
}

func splitLines(raw []byte) []string {
	trimmed := strings.TrimSpace(string(bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func writeAuthorizedKeysAtomic(path string, lines []string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".authorized_keys.*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func normalizeIdentifier(identifier string) string {
	if value := strings.TrimSpace(identifier); value != "" {
		return value
	}
	return DefaultIdentifier
}
