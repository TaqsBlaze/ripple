package pairing

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"ripple/internal/transfer"
)

func (s *HTTPServer) authenticatedTransferBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, Peer, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "request body is too large or unreadable", http.StatusBadRequest)
		return nil, Peer{}, false
	}
	peer, err := s.Manager.VerifyRequest(r.Header.Get("X-Ripple-Device-ID"), r.Header.Get("X-Ripple-Signature"), r.Method, r.URL.RequestURI(), body)
	if err != nil {
		http.Error(w, "device authentication failed", http.StatusUnauthorized)
		return nil, Peer{}, false
	}
	return body, peer, true
}

func (s *HTTPServer) offerTransfer(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	body, peer, ok := s.authenticatedTransferBody(w, r, 64<<10)
	if !ok {
		return
	}
	var offer struct {
		Version int    `json:"version"`
		ID      string `json:"transferId"`
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		MIME    string `json:"mimeType"`
		SHA256  string `json:"sha256"`
	}
	if json.Unmarshal(body, &offer) != nil || offer.Version != 1 {
		http.Error(w, "invalid transfer offer", http.StatusBadRequest)
		return
	}
	item := transfer.Item{ID: offer.ID, PeerID: peer.ID, PeerName: peer.Name, Name: offer.Name, Size: offer.Size, MIME: offer.MIME, SHA256: offer.SHA256}
	if err := s.Transfers.Offer(item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	state, _ := s.Transfers.Status(item.ID, peer.ID)
	if state.State == "offered" && s.Transfers.AutoAccept() {
		if err := s.Transfers.Decide(item.ID, true); err != nil {
			http.Error(w, "could not apply receive policy", http.StatusInternalServerError)
			return
		}
		state.State = "accepted"
	}
	s.respond(w, http.StatusCreated, map[string]string{"state": state.State})
}

func (s *HTTPServer) transferStatus(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	_, peer, ok := s.authenticatedTransferBody(w, r, 0)
	if !ok {
		return
	}
	item, err := s.Transfers.Status(r.PathValue("id"), peer.ID)
	if err != nil {
		http.Error(w, "transfer not found", http.StatusNotFound)
		return
	}
	s.respond(w, http.StatusOK, map[string]any{"state": item.State, "offset": item.Offset, "size": item.Size, "sha256": item.SHA256, "direction": item.Direction})
}

func (s *HTTPServer) transferInbox(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	_, peer, ok := s.authenticatedTransferBody(w, r, 0)
	if !ok {
		return
	}
	items := s.Transfers.OutgoingForPeer(peer.ID)
	views := make([]map[string]any, 0, len(items))
	for _, item := range items {
		views = append(views, map[string]any{"transferId": item.ID, "name": item.Name, "size": item.Size, "mimeType": item.MIME, "sha256": item.SHA256, "state": item.State, "offset": item.Offset, "createdAt": item.CreatedAt})
	}
	s.respond(w, http.StatusOK, map[string]any{"transfers": views})
}

func (s *HTTPServer) decideOutgoingTransfer(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	body, peer, ok := s.authenticatedTransferBody(w, r, 64<<10)
	if !ok {
		return
	}
	var request struct {
		Approve bool `json:"approve"`
	}
	if json.Unmarshal(body, &request) != nil {
		http.Error(w, "invalid transfer decision", http.StatusBadRequest)
		return
	}
	item, err := s.Transfers.DecideForPeer(r.PathValue("id"), peer.ID, request.Approve)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.respond(w, http.StatusOK, map[string]string{"state": item.State})
}

func (s *HTTPServer) transferContent(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	_, peer, ok := s.authenticatedTransferBody(w, r, 0)
	if !ok {
		return
	}
	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil {
		http.Error(w, "invalid content offset", http.StatusBadRequest)
		return
	}
	_, chunk, err := s.Transfers.ReadOutgoingChunk(r.PathValue("id"), peer.ID, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(chunk)
}

func (s *HTTPServer) transferReceipt(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	body, peer, ok := s.authenticatedTransferBody(w, r, 64<<10)
	if !ok {
		return
	}
	var receipt struct {
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}
	if json.Unmarshal(body, &receipt) != nil {
		http.Error(w, "invalid transfer receipt", http.StatusBadRequest)
		return
	}
	item, err := s.Transfers.CompleteOutgoing(r.PathValue("id"), peer.ID, receipt.Size, receipt.SHA256)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.respond(w, http.StatusOK, map[string]string{"state": item.State, "receiptId": item.ReceiptID, "sha256": item.SHA256})
}

func (s *HTTPServer) transferChunk(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	body, peer, ok := s.authenticatedTransferBody(w, r, transfer.ChunkSize)
	if !ok {
		return
	}
	offset, err := strconv.ParseInt(r.PathValue("offset"), 10, 64)
	if err != nil {
		http.Error(w, "invalid chunk offset", http.StatusBadRequest)
		return
	}
	item, err := s.Transfers.Append(r.PathValue("id"), peer.ID, offset, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.respond(w, http.StatusOK, map[string]int64{"offset": item.Offset})
}

func (s *HTTPServer) completeTransfer(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	body, peer, ok := s.authenticatedTransferBody(w, r, 64<<10)
	if !ok {
		return
	}
	var completion struct {
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}
	if json.Unmarshal(body, &completion) != nil {
		http.Error(w, "invalid completion request", http.StatusBadRequest)
		return
	}
	item, err := s.Transfers.Complete(r.PathValue("id"), peer.ID, completion.Size, completion.SHA256)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.respond(w, http.StatusOK, map[string]string{"state": item.State, "receiptId": item.ReceiptID, "sha256": item.SHA256})
}

func (s *HTTPServer) cancelTransfer(w http.ResponseWriter, r *http.Request) {
	if s.Transfers == nil {
		http.Error(w, "transfers unavailable", http.StatusServiceUnavailable)
		return
	}
	_, peer, ok := s.authenticatedTransferBody(w, r, 64<<10)
	if !ok {
		return
	}
	if err := s.Transfers.Cancel(r.PathValue("id"), peer.ID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.respond(w, http.StatusOK, map[string]string{"state": "cancelled"})
}

func (s *HTTPServer) dropState(w http.ResponseWriter, r *http.Request) {
	if s.DropState == nil {
		http.Error(w, "drop feedback unavailable", http.StatusServiceUnavailable)
		return
	}
	body, peer, ok := s.authenticatedTransferBody(w, r, 64<<10)
	if !ok {
		return
	}
	var request struct {
		Active bool `json:"active"`
	}
	if json.Unmarshal(body, &request) != nil {
		http.Error(w, "invalid drop state", http.StatusBadRequest)
		return
	}
	s.DropState.Set(peer.ID, request.Active)
	s.respond(w, http.StatusOK, map[string]bool{"active": request.Active})
}
