package pairing

import (
	"sync"
	"time"
)

// DropState carries short-lived, authenticated Android drag-hover feedback to the desktop UI.
type DropState struct {
	mu           sync.Mutex
	peers        map[string]time.Time
	desktopUntil time.Time
}

func NewDropState() *DropState { return &DropState{peers: make(map[string]time.Time)} }

func (s *DropState) Set(peerID string, active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !active {
		delete(s.peers, peerID)
		return
	}
	s.peers[peerID] = time.Now().Add(1500 * time.Millisecond)
}

func (s *DropState) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	active := false
	for peerID, expires := range s.peers {
		if !now.Before(expires) {
			delete(s.peers, peerID)
		} else {
			active = true
		}
	}
	return active
}

// SetDesktop tracks the desktop drag independently of Android hover feedback.
func (s *DropState) SetDesktop(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if active {
		s.desktopUntil = time.Now().Add(2 * time.Second)
	}
	// Keep a short pulse after release so a foreground Android poll can observe it.
}

func (s *DropState) DesktopActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.desktopUntil)
}
