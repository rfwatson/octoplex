package mediaserver

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTLSCert(t *testing.T) {
	keyPair, err := generateTLSCert("localhost", "rtmp.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, keyPair.Cert)
	require.NotEmpty(t, keyPair.Key)

	block, _ := pem.Decode(keyPair.Cert)
	require.NotNil(t, block, "failed to decode certificate PEM")

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	assert.Equal(t, "octoplex.netflux.io", cert.Subject.Organization[0])
	assert.Greater(t, cert.NotBefore, time.Now().Add(-time.Second), "not before should be in the future")
	assert.Greater(t, cert.NotAfter, time.Now().Add(4*365*24*time.Hour), "not after should be a long time in the future")

	// BitLen does not count leading zeroes, so the length will not always be 128 bits:
	assert.GreaterOrEqual(t, cert.SerialNumber.BitLen(), 100, "serial number should be around 128 bits")

	assert.True(t, cert.BasicConstraintsValid, "basic constraints should be valid")
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	assert.Contains(t, cert.DNSNames, "localhost", "DNS names should include localhost")
	assert.Contains(t, cert.DNSNames, "rtmp.example.com", "DNS names should include rtmp.example.com")

	block, _ = pem.Decode(keyPair.Key)
	require.NotNil(t, block, "failed to decode private key PEM")

	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	require.NoError(t, err)
	assert.IsType(t, &ecdsa.PrivateKey{}, privKey, "expected ECDSA private key")

	assert.True(t, privKey.PublicKey.Equal(cert.PublicKey), "private key should match the certificate's public key")
}
