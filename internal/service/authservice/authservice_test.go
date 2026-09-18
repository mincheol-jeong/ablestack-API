package authservice

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sergeymakinen/go-crypt/sha512"
)

func TestReadLinuxShadowHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shadow")
	content := "root:$6$root-hash:20000:0:99999:7:::\nlocked:!:20000:0:99999:7:::\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	hash, err := readLinuxShadowHash(path, "root")
	if err != nil {
		t.Fatal(err)
	}
	if hash != "$6$root-hash" {
		t.Fatalf("hash = %q", hash)
	}
	if _, err := readLinuxShadowHash(path, "locked"); err == nil {
		t.Fatal("locked account must be rejected")
	}
	if _, err := readLinuxShadowHash(path, "missing"); err == nil {
		t.Fatal("missing account must be rejected")
	}
}

func TestVerifyLinuxPasswordHashYescrypt(t *testing.T) {
	hash := "$y$j9T$e8R9q85ZuzUkArEUurdtS.$esON.7y6H.u3UCPVCpbRFueRpAut2n2cMf1EhpjbuiC"
	if !verifyLinuxPasswordHash(hash, "pleaseletmein") {
		t.Fatal("valid yescrypt password was rejected")
	}
	if verifyLinuxPasswordHash(hash, "wrong-password") {
		t.Fatal("invalid yescrypt password was accepted")
	}
}

func TestVerifyLinuxPasswordHashSHA512(t *testing.T) {
	hash, err := sha512.NewHash("correct-password", sha512.MinRounds)
	if err != nil {
		t.Fatal(err)
	}
	if !verifyLinuxPasswordHash(hash, "correct-password") {
		t.Fatal("valid SHA-512 password was rejected")
	}
	if verifyLinuxPasswordHash(hash, "wrong-password") {
		t.Fatal("invalid SHA-512 password was accepted")
	}
}
