package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type DeviceName struct {
	mu   sync.RWMutex
	path string
	name string
}

func NewDeviceName(dir, fallback string) (*DeviceName, error) {
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		fallback = "Ripple PC"
	}
	s := &DeviceName{path: filepath.Join(dir, "device-name.json"), name: fallback}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var v struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	if !valid(v.Name) {
		return nil, errors.New("invalid saved device name")
	}
	s.name = v.Name
	return s, nil
}
func valid(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > 64 || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (s *DeviceName) Get() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.name }
func (s *DeviceName) Set(name string) error {
	name = strings.TrimSpace(name)
	if !valid(name) {
		return errors.New("Enter a name of 1 to 64 characters without control characters.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		Name string `json:"name"`
	}{name})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".device-name-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.name = name
	return nil
}
