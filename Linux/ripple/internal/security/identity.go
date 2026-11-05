package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Identity struct {
	Certificate *x509.Certificate
	PrivateKey  *ecdsa.PrivateKey
	Fingerprint string
}

func LoadOrCreate(dataDir, deviceName string) (*Identity, error) {
	identityDir := filepath.Join(dataDir, "identity")
	if err := os.MkdirAll(identityDir, 0o700); err != nil {
		return nil, fmt.Errorf("create identity directory: %w", err)
	}
	keyPath := filepath.Join(identityDir, "identity-key.pem")
	certPath := filepath.Join(identityDir, "identity-cert.pem")
	identity, err := load(keyPath, certPath)
	if err == nil {
		if len(identity.Certificate.IPAddresses) == 0 && len(identity.Certificate.DNSNames) == 0 {
			if err := renewCertificate(certPath, identity.PrivateKey, deviceName); err != nil {
				return nil, err
			}
			return load(keyPath, certPath)
		}
		return identity, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return create(keyPath, certPath, deviceName)
}

func load(keyPath, certPath string) (*Identity, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyBlock, _ := pem.Decode(keyData)
	certBlock, _ := pem.Decode(certData)
	if keyBlock == nil || certBlock == nil {
		return nil, errors.New("stored identity is not valid PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse identity key: %w", err)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse identity certificate: %w", err)
	}
	publicKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok || !publicKey.Equal(&key.PublicKey) {
		return nil, errors.New("identity key does not match certificate")
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure identity key: %w", err)
	}
	return newIdentity(cert, key), nil
}

func create(keyPath, certPath, deviceName string) (*Identity, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate identity key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: sanitizeName(deviceName)},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{sanitizeName(deviceName)},
	}
	addInterfaceIPs(template)
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create identity certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal identity key: %w", err)
	}
	if err := writeExclusive(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, err
	}
	if err := writeExclusive(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o644); err != nil {
		_ = os.Remove(keyPath)
		return nil, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}
	return newIdentity(cert, key), nil
}

func renewCertificate(certPath string, key *ecdsa.PrivateKey, deviceName string) error {
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: sanitizeName(deviceName)}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(10, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, DNSNames: []string{sanitizeName(deviceName)}}
	addInterfaceIPs(template)
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	return os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

func addInterfaceIPs(template *x509.Certificate) {
	interfaces, _ := net.Interfaces()
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := item.Addrs()
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil {
				template.IPAddresses = append(template.IPAddresses, ip.To4())
			}
		}
	}
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func newIdentity(cert *x509.Certificate, key *ecdsa.PrivateKey) *Identity {
	digest := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	encoded := strings.ToUpper(hex.EncodeToString(digest[:]))
	groups := make([]string, 0, 8)
	for index := 0; index < len(encoded); index += 8 {
		groups = append(groups, encoded[index:index+8])
	}
	return &Identity{Certificate: cert, PrivateKey: key, Fingerprint: strings.Join(groups, " ")}
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Ripple Linux"
	}
	if len(name) > 64 {
		return name[:64]
	}
	return name
}
