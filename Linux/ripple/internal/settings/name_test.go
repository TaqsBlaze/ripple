package settings

import (
	"strings"
	"testing"
)

func TestDeviceNamePersistenceAndValidation(t *testing.T) {
	dir := t.TempDir()
	name, err := NewDeviceName(dir, "original")
	if err != nil {
		t.Fatal(err)
	}
	if err := name.Set("  Studio PC  "); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewDeviceName(dir, "fallback")
	if err != nil || reloaded.Get() != "Studio PC" {
		t.Fatalf("reloaded name = %q, error = %v", reloaded.Get(), err)
	}
	for _, invalid := range []string{" ", "bad\nname", strings.Repeat("a", 65)} {
		if err := name.Set(invalid); err == nil {
			t.Errorf("accepted invalid name %q", invalid)
		}
	}
	if name.Get() != "Studio PC" {
		t.Fatal("invalid input changed the name")
	}
}
