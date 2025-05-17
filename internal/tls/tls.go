package tls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
)

// GenerateKeyPair generates a self-signed TLS certificate and private key.
func GenerateKeyPair(dnsNames ...string) (domain.KeyPair, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return domain.KeyPair{}, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return domain.KeyPair{}, err
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"octoplex.netflux.io"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(5 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return domain.KeyPair{}, err
	}

	var certPEM, keyPEM bytes.Buffer

	if err = pem.Encode(&certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return domain.KeyPair{}, err
	}

	privKeyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return domain.KeyPair{}, err
	}

	if err := pem.Encode(&keyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privKeyDER}); err != nil {
		return domain.KeyPair{}, err
	}

	return domain.KeyPair{
		Cert: certPEM.Bytes(),
		Key:  keyPEM.Bytes(),
	}, nil
}
