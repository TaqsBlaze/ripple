package pairing

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Invitation struct {
	Version     int    `json:"version"`
	Endpoint    string `json:"endpoint"`
	Fingerprint string `json:"fingerprint"`
	ExpiresAt   string `json:"expiresAt"`
	SessionID   string `json:"sessionId"`
	Secret      string `json:"secret"`
}

type Peer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	DeviceID  string    `json:"deviceId"`
	PublicKey string    `json:"publicKey"`
	PairedAt  time.Time `json:"pairedAt"`
}

type candidate struct {
	EnrollmentID string
	SessionID    string
	DeviceID     string
	DeviceName   string
	PublicKey    []byte
	Challenge    []byte
	ExpiresAt    time.Time
	State        string
}

type Manager struct {
	mu         sync.Mutex
	path       string
	invitation *Invitation
	candidates map[string]*candidate
	peers      map[string]Peer
}

func New(dataDir string) (*Manager, error) {
	manager := &Manager{path: filepath.Join(dataDir, "peers.json"), candidates: make(map[string]*candidate), peers: make(map[string]Peer)}
	data, err := os.ReadFile(manager.path)
	if err == nil {
		var peers []Peer
		if err := json.Unmarshal(data, &peers); err != nil {
			return nil, fmt.Errorf("read trusted peers: %w", err)
		}
		for _, peer := range peers {
			manager.peers[peer.ID] = peer
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) CreateInvitation(endpoint, fingerprint string) (Invitation, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return Invitation{}, err
	}
	id, err := newID()
	if err != nil {
		return Invitation{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	invitation := Invitation{1, endpoint, strings.ToLower(fingerprint), time.Now().Add(2 * time.Minute).UTC().Format(time.RFC3339), id, base64.RawURLEncoding.EncodeToString(secret)}
	m.invitation = &invitation
	return invitation, nil
}

func (m *Manager) Start(session, secret, deviceID, deviceName string, publicKey []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv := m.invitation
	if inv == nil || inv.SessionID != session {
		return "", errors.New("invitation is unavailable")
	}
	expires, _ := time.Parse(time.RFC3339, inv.ExpiresAt)
	if time.Now().After(expires) {
		m.invitation = nil
		return "", errors.New("invitation expired")
	}
	provided, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil {
		return "", errors.New("invalid invitation secret")
	}
	expected, _ := base64.RawURLEncoding.DecodeString(inv.Secret)
	if len(provided) != len(expected) || subtleCompare(provided, expected) == false {
		return "", errors.New("invalid invitation secret")
	}
	if len(publicKey) > 1024 || len(deviceID) > 64 || len(deviceName) > 64 || strings.TrimSpace(deviceName) == "" {
		return "", errors.New("invalid device identity")
	}
	parsed, err := x509.ParsePKIXPublicKey(publicKey)
	if err != nil {
		return "", errors.New("invalid public key")
	}
	if _, ok := parsed.(*ecdsa.PublicKey); !ok {
		return "", errors.New("unsupported public key algorithm")
	}
	for _, existing := range m.candidates {
		if existing.SessionID == session && existing.DeviceID == deviceID {
			return existing.EnrollmentID, nil
		}
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	m.candidates[id] = &candidate{EnrollmentID: id, SessionID: session, DeviceID: deviceID, DeviceName: deviceName, PublicKey: publicKey, ExpiresAt: time.Now().Add(2 * time.Minute), State: "pending"}
	return id, nil
}

func (m *Manager) Status(session, enrollmentID string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.candidates[enrollmentID]
	if c == nil || c.SessionID != session {
		return nil, errors.New("unknown enrollment")
	}
	if time.Now().After(c.ExpiresAt) && c.State != "paired" {
		c.State = "expired"
	}
	result := map[string]string{"state": c.State}
	if c.State == "approved" {
		result["challenge"] = base64.StdEncoding.EncodeToString(c.Challenge)
	}
	return result, nil
}

func (m *Manager) Decide(id string, approve bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.candidates[id]
	if c == nil || c.State != "pending" || time.Now().After(c.ExpiresAt) {
		return errors.New("enrollment is no longer pending")
	}
	if !approve {
		c.State = "rejected"
		return nil
	}
	c.Challenge = make([]byte, 32)
	if _, err := rand.Read(c.Challenge); err != nil {
		return err
	}
	c.State = "approved"
	return nil
}

func (m *Manager) Prove(session, id, deviceID, signature string) (Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.candidates[id]
	if c == nil || c.SessionID != session || c.DeviceID != deviceID || c.State != "approved" || time.Now().After(c.ExpiresAt) {
		return Peer{}, errors.New("approval challenge is unavailable")
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return Peer{}, errors.New("invalid identity proof")
	}
	keyAny, err := x509.ParsePKIXPublicKey(c.PublicKey)
	if err != nil {
		return Peer{}, err
	}
	key := keyAny.(*ecdsa.PublicKey)
	digest := sha256.Sum256(c.Challenge)
	if !ecdsa.VerifyASN1(key, digest[:], sig) {
		return Peer{}, errors.New("identity proof failed")
	}
	peerDigest := sha256.Sum256(c.PublicKey)
	peerID := fmt.Sprintf("%x-%x-%x-%x-%x", peerDigest[0:4], peerDigest[4:6], peerDigest[6:8], peerDigest[8:10], peerDigest[10:16])
	peer := Peer{ID: peerID, Name: c.DeviceName, DeviceID: c.DeviceID, PublicKey: base64.StdEncoding.EncodeToString(c.PublicKey), PairedAt: time.Now().UTC()}
	m.peers[peer.ID] = peer
	if err := m.persist(); err != nil {
		delete(m.peers, peer.ID)
		return Peer{}, err
	}
	c.State = "paired"
	m.invitation = nil
	return peer, nil
}

func (m *Manager) Pending() []map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]map[string]string, 0)
	for _, c := range m.candidates {
		if c.State == "pending" && time.Now().Before(c.ExpiresAt) {
			result = append(result, map[string]string{"id": c.EnrollmentID, "name": c.DeviceName, "deviceId": c.DeviceID})
		}
	}
	return result
}

func (m *Manager) Peers() []Peer {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Peer, 0, len(m.peers))
	for _, peer := range m.peers {
		result = append(result, peer)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func (m *Manager) VerifyRequest(deviceID, signature, method, path string, body []byte) (Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var peer Peer
	for _, candidate := range m.peers {
		if candidate.DeviceID == deviceID {
			peer = candidate
			break
		}
	}
	if peer.ID == "" {
		return Peer{}, errors.New("untrusted device")
	}
	publicDER, err := base64.StdEncoding.DecodeString(peer.PublicKey)
	if err != nil {
		return Peer{}, err
	}
	keyAny, err := x509.ParsePKIXPublicKey(publicDER)
	if err != nil {
		return Peer{}, err
	}
	key, ok := keyAny.(*ecdsa.PublicKey)
	if !ok {
		return Peer{}, errors.New("unsupported peer key")
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return Peer{}, errors.New("invalid request signature")
	}
	bodyHash := sha256.Sum256(body)
	message := fmt.Sprintf("%s\n%s\n%x", method, path, bodyHash)
	digest := sha256.Sum256([]byte(message))
	if !ecdsa.VerifyASN1(key, digest[:], sig) {
		return Peer{}, errors.New("request signature failed")
	}
	return peer, nil
}

func (m *Manager) persist() error {
	peers := make([]Peer, 0, len(m.peers))
	for _, peer := range m.peers {
		peers = append(peers, peer)
	}
	data, err := json.Marshal(peers)
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func subtleCompare(a, b []byte) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
