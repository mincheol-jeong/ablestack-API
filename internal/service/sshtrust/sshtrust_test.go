package sshtrust

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestEnsureAuthorizedKeyPreservesOtherEntriesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authorized_keys")
	other := testPublicKey(t, "operator@host")
	oldCloud := testPublicKey(t, DefaultIdentifier)
	newCloud := testPublicKey(t, DefaultIdentifier)
	if err := os.WriteFile(path, []byte(other+"\n"+oldCloud+"\n"+newCloud+"\ninvalid preserved line\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := EnsureAuthorizedKey(path, newCloud, DefaultIdentifier)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || !result.Present {
		t.Fatalf("unexpected result: %#v", result)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, other) || !strings.Contains(content, "invalid preserved line") {
		t.Fatalf("unrelated entries were not preserved: %s", content)
	}
	if strings.Count(content, DefaultIdentifier) != 1 || !strings.Contains(content, newCloud) {
		t.Fatalf("cloud key was not replaced exactly once: %s", content)
	}

	result, err = EnsureAuthorizedKey(path, newCloud, DefaultIdentifier)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Fatalf("second ensure must be idempotent: %#v", result)
	}
}

func TestRemoveAuthorizedKeyOnlyRemovesCloudEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authorized_keys")
	other := testPublicKey(t, "operator@host")
	cloud := testPublicKey(t, DefaultIdentifier)
	if err := os.WriteFile(path, []byte(other+"\n"+cloud+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RemoveAuthorizedKey(path, DefaultIdentifier)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Present {
		t.Fatalf("unexpected result: %#v", result)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != other+"\n" {
		t.Fatalf("unexpected authorized_keys: %q", raw)
	}
}

func testPublicKey(t *testing.T, comment string) string {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))) + " " + comment
}
