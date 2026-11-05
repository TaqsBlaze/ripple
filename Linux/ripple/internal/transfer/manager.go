package transfer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const ChunkSize = 1 << 20
const MaxTransferSize = int64(10 << 30)

type Item struct {
	ID              string `json:"id"`
	PeerID          string `json:"peerId"`
	PeerName        string `json:"peerName"`
	Name            string `json:"name"`
	MIME            string `json:"mimeType"`
	Size            int64  `json:"size"`
	SHA256          string `json:"sha256"`
	Offset          int64  `json:"offset"`
	LastChunkOffset int64  `json:"lastChunkOffset,omitempty"`
	LastChunkSize   int64  `json:"lastChunkSize,omitempty"`
	State           string `json:"state"`
	Direction       string `json:"direction,omitempty"`
	SourcePath      string `json:"sourcePath,omitempty"`
	SourceModTime   int64  `json:"sourceModTime,omitempty"`
	ReceiptID       string `json:"receiptId,omitempty"`
	Deleted         bool   `json:"deleted,omitempty"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
}
type Manager struct {
	mu         sync.Mutex
	root       string
	items      map[string]*Item
	autoAccept bool
}

func New(root string) (*Manager, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{root: root, items: map[string]*Item{}}
	data, err := os.ReadFile(filepath.Join(root, "transfers.json"))
	if err == nil {
		var items []*Item
		if err = json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Direction == "" {
				item.Direction = "receive"
			}
			if item.CreatedAt == 0 {
				item.CreatedAt = item.UpdatedAt
			}
			m.items[item.ID] = item
			if item.Deleted {
				_ = os.Remove(filepath.Join(root, item.Name))
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	policy, err := os.ReadFile(filepath.Join(root, "receive-policy.json"))
	if err == nil {
		var value struct {
			AutoAccept bool `json:"autoAccept"`
		}
		if err := json.Unmarshal(policy, &value); err != nil {
			return nil, err
		}
		m.autoAccept = value.AutoAccept
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return m, nil
}

func (m *Manager) AutoAccept() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.autoAccept
}

func (m *Manager) SetAutoAccept(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(struct {
		AutoAccept bool `json:"autoAccept"`
	}{enabled})
	if err != nil {
		return err
	}
	path := filepath.Join(m.root, "receive-policy.json")
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	m.autoAccept = enabled
	return nil
}

func (m *Manager) FilePath(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.Direction != "receive" || item.State != "completed" || item.Deleted {
		return "", errors.New("received file is not complete")
	}
	return filepath.Join(m.root, item.Name), nil
}

func (m *Manager) DeleteCompleted(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.Direction != "receive" || item.State != "completed" {
		return errors.New("received file is not complete")
	}
	if item.Deleted {
		return nil
	}
	item.Deleted = true
	item.UpdatedAt = time.Now().UnixMilli()
	if err := m.persist(); err != nil {
		item.Deleted = false
		return err
	}
	if err := os.Remove(filepath.Join(m.root, item.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		item.Deleted = false
		_ = m.persist()
		return err
	}
	return nil
}
func (m *Manager) Offer(item Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !validTransferID(item.ID) || item.Size < 0 || item.Size > MaxTransferSize || len(item.Name) > 255 || len(item.SHA256) != 64 {
		return errors.New("invalid transfer offer")
	}
	if item.Direction == "" {
		item.Direction = "receive"
	}
	if item.Direction != "receive" && item.Direction != "send" {
		return errors.New("invalid transfer direction")
	}
	if existing, ok := m.items[item.ID]; ok {
		if existing.PeerID == item.PeerID && existing.Direction == item.Direction && existing.Name == item.Name && existing.Size == item.Size && existing.SHA256 == item.SHA256 && existing.SourcePath == item.SourcePath {
			return nil
		}
		return errors.New("transfer ID is already in use")
	}
	item.Name = safeName(item.Name)
	item.State = "offered"
	item.UpdatedAt = time.Now().UnixMilli()
	item.CreatedAt = item.UpdatedAt
	m.items[item.ID] = &item
	if err := m.persist(); err != nil {
		delete(m.items, item.ID)
		return err
	}
	return nil
}

// OfferLocalFile creates a durable outbound transfer for a local drag-and-drop file.
// File bytes remain in the Linux service and are streamed to the authenticated peer.
func (m *Manager) OfferLocalFile(peerID, peerName, sourcePath string) (Item, error) {
	path, err := filepath.Abs(sourcePath)
	if err != nil {
		return Item{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Item{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 0 || info.Size() > MaxTransferSize {
		return Item{}, errors.New("choose a regular file up to 10 GiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return Item{}, err
	}
	hash := sha256.New()
	count, copyErr := io.Copy(hash, file)
	openedInfo, statErr := file.Stat()
	closeErr := file.Close()
	if copyErr != nil {
		return Item{}, copyErr
	}
	if statErr != nil {
		return Item{}, statErr
	}
	if closeErr != nil {
		return Item{}, closeErr
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, openedInfo) || !os.SameFile(openedInfo, after) || after.Mode()&os.ModeSymlink != 0 || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || count != info.Size() {
		return Item{}, errors.New("file changed while Ripple was preparing it")
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Item{}, err
	}
	id := fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
	item := Item{ID: id, PeerID: peerID, PeerName: peerName, Name: info.Name(), MIME: mime.TypeByExtension(filepath.Ext(path)), Size: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil)), Direction: "send", SourcePath: path, SourceModTime: info.ModTime().UnixNano()}
	if parsed, _, parseErr := mime.ParseMediaType(item.MIME); parseErr == nil {
		item.MIME = parsed
	} else {
		item.MIME = "application/octet-stream"
	}
	if err := m.Offer(item); err != nil {
		return Item{}, err
	}
	return m.Status(item.ID, peerID)
}

func (m *Manager) DecideForPeer(id, peerID string, approve bool) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.PeerID != peerID || item.Direction != "send" || item.State != "offered" {
		return Item{}, errors.New("transfer is no longer awaiting this device's decision")
	}
	if approve {
		item.State = "accepted"
	} else {
		item.State = "rejected"
	}
	item.UpdatedAt = time.Now().UnixMilli()
	if err := m.persist(); err != nil {
		return Item{}, err
	}
	return *item, nil
}

func (m *Manager) OutgoingForPeer(peerID string) []Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Item, 0)
	for _, item := range m.items {
		if item.PeerID == peerID && item.Direction == "send" && !item.Deleted {
			out = append(out, *item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out
}

func (m *Manager) ReadOutgoingChunk(id, peerID string, offset int64) (Item, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.PeerID != peerID || item.Direction != "send" {
		return Item{}, nil, errors.New("transfer not found")
	}
	if item.State != "accepted" && item.State != "transferring" {
		return Item{}, nil, errors.New("transfer has not been accepted")
	}
	if offset < 0 || offset >= item.Size {
		return Item{}, nil, errors.New("invalid source offset")
	}
	retry := item.LastChunkSize > 0 && offset == item.LastChunkOffset
	if offset != item.Offset && !retry {
		return Item{}, nil, errors.New("source offset is not the next contiguous chunk")
	}
	info, err := os.Lstat(item.SourcePath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != item.Size || info.ModTime().UnixNano() != item.SourceModTime {
		return Item{}, nil, errors.New("source file changed or is unavailable")
	}
	length := int64(ChunkSize)
	if remain := item.Size - offset; remain < length {
		length = remain
	}
	file, err := os.Open(item.SourcePath)
	if err != nil {
		return Item{}, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return Item{}, nil, errors.New("source file changed or is unavailable")
	}
	data := make([]byte, length)
	count, readErr := file.ReadAt(data, offset)
	closeErr := file.Close()
	if readErr != nil && readErr != io.EOF {
		return Item{}, nil, readErr
	}
	if closeErr != nil {
		return Item{}, nil, closeErr
	}
	data = data[:count]
	if int64(count) != length {
		return Item{}, nil, io.ErrUnexpectedEOF
	}
	if !retry {
		item.LastChunkOffset = offset
		item.LastChunkSize = int64(count)
		item.Offset = offset + int64(count)
		item.State = "transferring"
		item.UpdatedAt = time.Now().UnixMilli()
		if err := m.persist(); err != nil {
			return Item{}, nil, err
		}
	}
	return *item, data, nil
}

func (m *Manager) CompleteOutgoing(id, peerID string, size int64, digest string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.PeerID != peerID || item.Direction != "send" {
		return Item{}, errors.New("transfer not found")
	}
	if item.State == "completed" {
		return *item, nil
	}
	if item.State != "accepted" && item.State != "transferring" {
		return Item{}, errors.New("transfer was not accepted")
	}
	if size != item.Size || digest != item.SHA256 || item.Offset != size {
		return Item{}, errors.New("receipt does not match the transfer")
	}
	info, err := os.Lstat(item.SourcePath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != item.Size || info.ModTime().UnixNano() != item.SourceModTime {
		return Item{}, errors.New("source file changed or is unavailable")
	}
	file, err := os.Open(item.SourcePath)
	if err != nil {
		return Item{}, err
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return Item{}, errors.New("source file changed or is unavailable")
	}
	hash := sha256.New()
	count, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return Item{}, copyErr
	}
	if closeErr != nil {
		return Item{}, closeErr
	}
	if count != item.Size || hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
		return Item{}, errors.New("source file changed during transfer")
	}
	item.State = "completed"
	item.ReceiptID = id
	item.UpdatedAt = time.Now().UnixMilli()
	if err := m.persist(); err != nil {
		return Item{}, err
	}
	return *item, nil
}

func validTransferID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(id, "-", "")
	_, err := hex.DecodeString(compact)
	return err == nil
}
func (m *Manager) Decide(id string, approve bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.Direction != "receive" || item.State != "offered" {
		return errors.New("transfer is no longer awaiting approval")
	}
	if approve {
		item.State = "accepted"
	} else {
		item.State = "rejected"
	}
	item.UpdatedAt = time.Now().UnixMilli()
	return m.persist()
}
func (m *Manager) Status(id, peerID string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.PeerID != peerID {
		return Item{}, errors.New("transfer not found")
	}
	return *item, nil
}
func (m *Manager) Append(id, peerID string, offset int64, body []byte) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.Direction != "receive" || item.PeerID != peerID {
		return Item{}, errors.New("transfer not found")
	}
	if item.State != "accepted" && item.State != "transferring" {
		return Item{}, errors.New("transfer is not accepted")
	}
	if len(body) == 0 || len(body) > ChunkSize || offset < 0 {
		return Item{}, errors.New("invalid chunk")
	}
	if offset < item.Offset {
		return Item{}, errors.New("duplicate chunk cannot be verified")
	}
	if offset != item.Offset || offset+int64(len(body)) > item.Size {
		return Item{}, errors.New("chunk offset or length is invalid")
	}
	file, err := os.OpenFile(filepath.Join(m.root, item.ID+".part"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Item{}, err
	}
	if _, err = file.Seek(offset, io.SeekStart); err == nil {
		_, err = file.Write(body)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return Item{}, err
	}
	item.Offset += int64(len(body))
	item.State = "transferring"
	item.UpdatedAt = time.Now().UnixMilli()
	if err = m.persist(); err != nil {
		return Item{}, err
	}
	return *item, nil
}
func (m *Manager) Complete(id, peerID string, size int64, digest string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.Direction != "receive" || item.PeerID != peerID {
		return Item{}, errors.New("transfer not found")
	}
	if item.State == "completed" {
		return *item, nil
	}
	if item.Offset != size || size != item.Size || digest != item.SHA256 {
		return Item{}, errors.New("completion metadata does not match offer")
	}
	filePath := filepath.Join(m.root, id+".part")
	if size == 0 {
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return Item{}, err
		}
		if err = file.Sync(); err != nil {
			_ = file.Close()
			return Item{}, err
		}
		if err = file.Close(); err != nil {
			return Item{}, err
		}
	}
	file, err := os.Open(filePath)
	if err != nil {
		return Item{}, err
	}
	hash := sha256.New()
	count, err := io.Copy(hash, file)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return Item{}, err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if count != size || actual != item.SHA256 {
		item.State = "failed"
		item.UpdatedAt = time.Now().UnixMilli()
		_ = m.persist()
		return Item{}, errors.New("received file failed length or SHA-256 verification")
	}
	dest, err := m.safeDestination(item.Name)
	if err != nil {
		return Item{}, err
	}
	if err = os.Rename(filePath, dest); err != nil {
		return Item{}, err
	}
	item.Name = filepath.Base(dest)
	item.State = "completed"
	item.ReceiptID = id
	item.UpdatedAt = time.Now().UnixMilli()
	if err = m.persist(); err != nil {
		return Item{}, err
	}
	return *item, nil
}
func (m *Manager) Cancel(id, peerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]
	if item == nil || item.PeerID != peerID {
		return errors.New("transfer not found")
	}
	if item.State == "completed" || item.State == "rejected" {
		return errors.New("transfer is terminal")
	}
	item.State = "cancelled"
	item.UpdatedAt = time.Now().UnixMilli()
	_ = os.Remove(filepath.Join(m.root, id+".part"))
	return m.persist()
}
func (m *Manager) Pending() []Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Item{}
	for _, item := range m.items {
		if item.State == "offered" {
			out = append(out, *item)
		}
	}
	return out
}
func (m *Manager) List() []Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Item, 0, len(m.items))
	for _, item := range m.items {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out
}
func (m *Manager) persist() error {
	out := make([]*Item, 0, len(m.items))
	for _, item := range m.items {
		out = append(out, item)
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	tmp := filepath.Join(m.root, "transfers.json.tmp")
	if err = os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(m.root, "transfers.json"))
}
func (m *Manager) safeDestination(name string) (string, error) {
	base := safeName(name)
	for i := 0; i < 1000; i++ {
		candidate := base
		if i > 0 {
			ext := filepath.Ext(base)
			candidate = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(base, ext), i, ext)
		}
		path := filepath.Join(m.root, candidate)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
	}
	return "", errors.New("could not find a collision-safe destination name")
}
func safeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "received-file"
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
