package pairing

import (
	"testing"
	"time"
)

func TestDesktopDropFeedbackIsSeparateAndExpires(t *testing.T) {
	state := NewDropState()
	state.Set("phone", true)
	if state.DesktopActive() {
		t.Fatal("phone feedback must not activate desktop feedback")
	}
	state.SetDesktop(true)
	state.Set("phone", false)
	if !state.DesktopActive() || state.Active() {
		t.Fatal("desktop and phone feedback must be independent")
	}
	state.SetDesktop(false)
	if !state.DesktopActive() {
		t.Fatal("short desktop pulse should remain observable after release")
	}
	state.mu.Lock()
	state.desktopUntil = time.Now().Add(-time.Second)
	state.mu.Unlock()
	if state.DesktopActive() {
		t.Fatal("desktop feedback must expire")
	}
}
