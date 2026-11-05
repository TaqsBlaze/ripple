package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestReceiveDirectoryPersistsAndKeepsExistingFiles(t *testing.T) {
	root := t.TempDir()
	first, second := t.TempDir(), t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetReceiveDir(first); err != nil {
		t.Fatal(err)
	}
	if err := m.SetAutoAccept(true); err != nil {
		t.Fatal(err)
	}
	data := []byte("hello")
	sum := sha256.Sum256(data)
	id := "01234567-89ab-cdef-0123-456789abcdef"
	if err := m.Offer(Item{ID: id, PeerID: "peer", Name: "hello.txt", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide(id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Append(id, "peer", 0, data); err != nil {
		t.Fatal(err)
	}
	if err := m.SetReceiveDir(second); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Complete(id, "peer", int64(len(data)), hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	file, err := m.FilePath(id)
	if err != nil || file != filepath.Join(first, "hello.txt") {
		t.Fatalf("file path %q: %v", file, err)
	}
	if contents, err := os.ReadFile(file); err != nil || string(contents) != string(data) {
		t.Fatalf("file contents %q: %v", contents, err)
	}
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ReceiveDir() != second || !reopened.AutoAccept() {
		t.Fatal("settings were not persisted together")
	}
	if path, err := reopened.FilePath(id); err != nil || path != file {
		t.Fatalf("old file path %q: %v", path, err)
	}
	if err := reopened.DeleteCompleted(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("old received file was not deleted: %v", err)
	}
	if err := reopened.SetReceiveDir(filepath.Join(second, "missing")); err == nil {
		t.Fatal("missing folder accepted")
	}
	if reopened.ReceiveDir() != second {
		t.Fatal("invalid folder changed the setting")
	}
}
