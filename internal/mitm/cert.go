package mitm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CertManager handles Root CA generation and per-domain leaf certificate issuance.
type CertManager struct {
	dir      string
	mu       sync.RWMutex
	rootKey  *ecdsa.PrivateKey
	rootCert *x509.Certificate
	cache    map[string]*tls.Certificate
}

func NewCertManager(dir string) *CertManager {
	return &CertManager{
		dir:   dir,
		cache: make(map[string]*tls.Certificate),
	}
}

func (cm *CertManager) rootCAKeyPath() string  { return filepath.Join(cm.dir, "rootCA.key") }
func (cm *CertManager) rootCACertPath() string { return filepath.Join(cm.dir, "rootCA.crt") }

// EnsureRootCA loads or generates the Root CA certificate.
func (cm *CertManager) EnsureRootCA() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.tryLoad() {
		return nil
	}
	return cm.generate()
}

func (cm *CertManager) tryLoad() bool {
	keyPEM, err := os.ReadFile(cm.rootCAKeyPath())
	if err != nil {
		return false
	}
	certPEM, err := os.ReadFile(cm.rootCACertPath())
	if err != nil {
		return false
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return false
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return false
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return false
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return false
	}

	// Check expiry (30-day threshold)
	if time.Now().Add(30 * 24 * time.Hour).After(cert.NotAfter) {
		return false
	}

	cm.rootKey = key
	cm.rootCert = cert
	return true
}

func (cm *CertManager) generate() error {
	if err := os.MkdirAll(cm.dir, 0o700); err != nil {
		return err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Cyrene Gateway MITM Root CA",
			Organization: []string{"Cyrene Gateway"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}

	// Write key
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(cm.rootCAKeyPath(), keyPEM, 0o600); err != nil {
		return err
	}

	// Write cert
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(cm.rootCACertPath(), certPEM, 0o644); err != nil {
		return err
	}

	cm.rootKey = key
	cm.rootCert = cert
	return nil
}

// GetCertificate returns a TLS certificate for the given SNI domain (generates if needed).
func (cm *CertManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := hello.ServerName
	if domain == "" {
		domain = "localhost"
	}

	cm.mu.RLock()
	if cert, ok := cm.cache[domain]; ok {
		cm.mu.RUnlock()
		return cert, nil
	}
	cm.mu.RUnlock()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock
	if cert, ok := cm.cache[domain]; ok {
		return cert, nil
	}

	cert, err := cm.generateLeaf(domain)
	if err != nil {
		return nil, err
	}
	cm.cache[domain] = cert
	return cert, nil
}

func (cm *CertManager) generateLeaf(domain string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{domain, "*." + domain},
	}

	// If domain is an IP, add to IPAddresses
	if ip := net.ParseIP(domain); ip != nil {
		template.IPAddresses = []net.IP{ip}
		template.DNSNames = nil
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, cm.rootCert, &key.PublicKey, cm.rootKey)
	if err != nil {
		return nil, err
	}

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}, nil
}

// RootCACertPEM returns the Root CA certificate in PEM format for download.
func (cm *CertManager) RootCACertPEM() ([]byte, error) {
	return os.ReadFile(cm.rootCACertPath())
}

// RootCACertPath returns the filesystem path to the Root CA certificate.
func (cm *CertManager) RootCACertPath() string {
	return cm.rootCACertPath()
}

// CertExists returns whether the Root CA certificate file exists.
func (cm *CertManager) CertExists() bool {
	_, err := os.Stat(cm.rootCACertPath())
	return err == nil
}
