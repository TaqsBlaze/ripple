//go:build linux

package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/cybergarage/go-mdns/mdns"
	"ripple/internal/ipc"
	"ripple/internal/pairing"
	"ripple/internal/protocol"
	"ripple/internal/security"
	"ripple/internal/settings"
	"ripple/internal/transfer"
)

var version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		slog.Error("Ripple service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	runtimeDirFlag := flag.String("runtime-dir", "", "override the user runtime directory")
	dataDirFlag := flag.String("data-dir", "", "override the user data directory")
	flag.Parse()
	runtimeDir, err := resolveRuntimeDir(*runtimeDirFlag)
	if err != nil {
		return err
	}
	dataDir, err := resolveDataDir(*dataDirFlag)
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	deviceName, err := settings.NewDeviceName(dataDir, hostname)
	if err != nil {
		return fmt.Errorf("load device name: %w", err)
	}
	identity, err := security.LoadOrCreate(dataDir, hostname)
	if err != nil {
		return fmt.Errorf("load device identity: %w", err)
	}
	pairs, err := pairing.New(dataDir)
	if err != nil {
		return fmt.Errorf("load paired devices: %w", err)
	}
	transfers, err := transfer.New(filepath.Join(dataDir, "received"))
	if err != nil {
		return fmt.Errorf("initialize transfer store: %w", err)
	}
	dropState := pairing.NewDropState()
	serverDigest := sha256.Sum256(identity.Certificate.RawSubjectPublicKeyInfo)
	serverID := hex.EncodeToString(serverDigest[:16])
	networkReady := false
	advertiser := mdns.NewServer()
	if err := advertiser.Start(); err != nil {
		slog.Warn("mDNS discovery could not start; QR pairing remains available", "error", err)
	} else {
		instance := "ripple-" + serverID[:12]
		err := advertiser.Register(context.Background(), &mdns.LocalService{
			Instance: instance,
			Service:  "_ripple._tcp",
			Host:     instance,
			Port:     43821,
			TXT:      []string{"version=1", "deviceId=" + serverID},
		})
		if err != nil {
			slog.Warn("mDNS discovery could not publish; QR pairing remains available", "error", err)
			_ = advertiser.Stop()
		} else {
			networkReady = true
			defer advertiser.Stop()
		}
	}
	certificate := tls.Certificate{Certificate: [][]byte{identity.Certificate.Raw}, PrivateKey: identity.PrivateKey}
	remote := &pairing.HTTPServer{Manager: pairs, Transfers: transfers, DropState: dropState, DeviceName: deviceName, Certificate: certificate, Port: 43821}
	server := &ipc.Server{
		SocketPath: filepath.Join(runtimeDir, "ripple", "ripple.sock"),
		Pairing:    pairs,
		Transfers:  transfers,
		DropState:  dropState,
		DeviceName: deviceName,
		Invitation: func() (pairing.Invitation, error) {
			digest := sha256.Sum256(identity.Certificate.RawSubjectPublicKeyInfo)
			return pairs.CreateInvitation(fmt.Sprintf("https://%s:43821", pairing.LANAddress()), hex.EncodeToString(digest[:]))
		},
		Status: protocol.ServiceStatus{
			ServiceVersion:      version,
			DeviceName:          hostname,
			IdentityFingerprint: identity.Fingerprint,
			NetworkReady:        networkReady,
			PairingReady:        true,
			TransferReady:       true,
		},
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = server.Close()
		_ = remote.Close()
	}()
	go func() {
		if err := remote.ListenAndServe(); err != nil {
			slog.Error("Pairing HTTPS listener stopped", "error", err)
			stop()
		}
	}()
	slog.Info("Ripple service ready", "socket", server.SocketPath, "fingerprint", identity.Fingerprint)
	return server.ListenAndServe()
}

func resolveRuntimeDir(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	if value := os.Getenv("XDG_RUNTIME_DIR"); value != "" {
		return value, nil
	}
	return "", errors.New("XDG_RUNTIME_DIR is unavailable; pass --runtime-dir for development")
}

func resolveDataDir(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	if value := os.Getenv("XDG_DATA_HOME"); value != "" {
		return filepath.Join(value, "ripple"), nil
	}
	value, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(value, ".local", "share", "ripple"), nil
}
