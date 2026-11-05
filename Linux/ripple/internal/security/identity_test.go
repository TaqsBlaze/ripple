package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityPersistsAndKeyIsPrivate(t *testing.T) {
	directory := t.TempDir()
	first, err := LoadOrCreate(directory, "Test PC")
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreate(directory, "Renamed PC")
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatal("identity changed between loads")
	}
	info, err := os.Stat(filepath.Join(directory, "identity", "identity-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("key mode is %o, want 600", got)
	}
}
