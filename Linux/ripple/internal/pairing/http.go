package pairing

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"ripple/internal/settings"
	"ripple/internal/transfer"
)

type HTTPServer struct {
	DeviceName  *settings.DeviceName
	Manager     *Manager
	Transfers   *transfer.Manager
	DropState   *DropState
	Certificate tls.Certificate
	Port        int
	server      *http.Server
}

func (s *HTTPServer) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/enrollment/start", s.start)
	mux.HandleFunc("/v1/enrollment/status", s.status)
	mux.HandleFunc("/v1/enrollment/proof", s.proof)
	mux.HandleFunc("/v1/peers/status", s.peerStatus)
	mux.HandleFunc("POST /v1/transfers", s.offerTransfer)
	mux.HandleFunc("GET /v1/transfers/inbox", s.transferInbox)
	mux.HandleFunc("POST /v1/transfers/{id}/decision", s.decideOutgoingTransfer)
	mux.HandleFunc("GET /v1/transfers/{id}", s.transferStatus)
	mux.HandleFunc("GET /v1/transfers/{id}/content", s.transferContent)
	mux.HandleFunc("POST /v1/transfers/{id}/receipt", s.transferReceipt)
	mux.HandleFunc("PUT /v1/transfers/{id}/chunks/{offset}", s.transferChunk)
	mux.HandleFunc("POST /v1/transfers/{id}/complete", s.completeTransfer)
	mux.HandleFunc("POST /v1/transfers/{id}/cancel", s.cancelTransfer)
	mux.HandleFunc("POST /v1/drop-state", s.dropState)
	s.server = &http.Server{Addr: fmt.Sprintf(":%d", s.Port), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{Certificates: []tls.Certificate{s.Certificate}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}}
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	return s.server.ServeTLS(listener, "", "")
}

func (s *HTTPServer) deviceName() string {
	if s.DeviceName == nil {
		return "Ripple PC"
	}
	return s.DeviceName.Get()
}

func (s *HTTPServer) peerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.ContentLength > 0 {
		http.Error(w, "unexpected request body", http.StatusBadRequest)
		return
	}
	peer, err := s.Manager.VerifyRequest(r.Header.Get("X-Ripple-Device-ID"), r.Header.Get("X-Ripple-Signature"), r.Method, r.URL.Path, nil)
	if err != nil {
		http.Error(w, "device authentication failed", http.StatusUnauthorized)
		return
	}
	s.respond(w, http.StatusOK, map[string]any{"version": 1, "state": "online", "peerId": peer.ID, "deviceName": s.deviceName()})
}
func (s *HTTPServer) Close() error {
	if s.server == nil {
		return nil
	}
	return s.server.Close()
}
func (s *HTTPServer) decode(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return nil, false
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return nil, false
	}
	return value, true
}
func (s *HTTPServer) respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (s *HTTPServer) start(w http.ResponseWriter, r *http.Request) {
	v, ok := s.decode(w, r)
	if !ok {
		return
	}
	if v["version"] != float64(1) {
		http.Error(w, "unsupported version", 400)
		return
	}
	publicKey, _ := base64.StdEncoding.DecodeString(stringValue(v, "publicKey"))
	id, err := s.Manager.Start(stringValue(v, "sessionId"), stringValue(v, "secret"), stringValue(v, "deviceId"), stringValue(v, "deviceName"), publicKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	s.respond(w, 200, map[string]string{"enrollmentId": id})
}
func (s *HTTPServer) status(w http.ResponseWriter, r *http.Request) {
	v, ok := s.decode(w, r)
	if !ok {
		return
	}
	state, err := s.Manager.Status(stringValue(v, "sessionId"), stringValue(v, "enrollmentId"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	s.respond(w, 200, state)
}
func (s *HTTPServer) proof(w http.ResponseWriter, r *http.Request) {
	v, ok := s.decode(w, r)
	if !ok {
		return
	}
	peer, err := s.Manager.Prove(stringValue(v, "sessionId"), stringValue(v, "enrollmentId"), stringValue(v, "deviceId"), stringValue(v, "signature"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	s.respond(w, 200, map[string]string{"state": "paired", "peerId": peer.ID, "deviceName": s.deviceName()})
}
func stringValue(m map[string]any, key string) string {
	v, _ := m[key].(string)
	if len(v) > 4096 {
		return ""
	}
	return strings.TrimSpace(v)
}

func LANAddress() string {
	interfaces, _ := net.Interfaces()
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := item.Addrs()
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}
func CertificateHasIP(cert *x509.Certificate, ip net.IP) bool {
	if cert == nil || ip == nil {
		return false
	}
	for _, candidate := range cert.IPAddresses {
		if candidate.Equal(ip) {
			return true
		}
	}
	return false
}

var _ = errors.New
