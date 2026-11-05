package pairing

import (
	"sync"
	"time"
)

// DropState carries short-lived, authenticated Android drag-hover feedback to the desktop UI.
type DropState struct {
	mu    sync.Mutex
	peers map[string]time.Time
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
