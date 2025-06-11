package server

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/tls"
)

// generateKeyPairs builds TLS key pairs for the given custom TLS configuration.
//
// It generates a self-signed certificate (valid for localhost, and any
// provided customHost unless a custom key pair is provided) and loads any
// optionally provided custom TLS certs (which must be valid for customHost, if
// provided).
//
// TODO: verify that the custom cert is valid for the custom host
func generateKeyPairs(customHost string, tlsCfg *config.TLS, dataDir string) (_ domain.KeyPairs, err error) {
	dnsNames := []string{"localhost"}
	if customHost != "" && tlsCfg == nil {
		dnsNames = append(dnsNames, customHost)
	}

	suffix := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(dnsNames, ","))))
	tlsDir := filepath.Join(dataDir, "tls")
	certPathInternal := filepath.Join(tlsDir, "cert_internal_"+suffix+".pem")
	keyPathInternal := filepath.Join(tlsDir, "key_internal_"+suffix+".pem")

	// First handle the internal key pair.
	// This is used for all TLS connections if no custom key pair is provided,
	// and some internal connections regardless.
	// It is generated only once for a given set of DNS names, and stored in the
	// data directory.
	var certAndKeyExist bool
	var keyPairInternal domain.KeyPair
	if certAndKeyExist, err = filesExist(certPathInternal, keyPathInternal); err != nil {
		return domain.KeyPairs{}, fmt.Errorf("check files exist: %w", err)
	} else if certAndKeyExist {
		if keyPairInternal, err = readKeyPair(certPathInternal, keyPathInternal); err != nil {
			return domain.KeyPairs{}, fmt.Errorf("read internal key pair: %w", err)
		}
	} else {
		if keyPairInternal, err = tls.GenerateKeyPair(dnsNames...); err != nil {
			return domain.KeyPairs{}, fmt.Errorf("generate cert: %w", err)
		}
		if err = os.MkdirAll(tlsDir, 0700); err != nil {
			return domain.KeyPairs{}, fmt.Errorf("create TLS directory: %w", err)
		}
		if err = writeKeyPair(keyPairInternal, certPathInternal, keyPathInternal); err != nil {
			return domain.KeyPairs{}, fmt.Errorf("write internal key pair: %w", err)
		}
	}

	// Now handle the custom key pair, if these are provided.
	var keyPairCustom domain.KeyPair
	if tlsCfg != nil {
		keyPairCustom.Cert, err = os.ReadFile(tlsCfg.CertPath)
		if err != nil {
			return domain.KeyPairs{}, fmt.Errorf("read custom cert: %w", err)
		}
		keyPairCustom.Key, err = os.ReadFile(tlsCfg.KeyPath)
		if err != nil {
			return domain.KeyPairs{}, fmt.Errorf("read custom key: %w", err)
		}
	}

	return domain.KeyPairs{
		Internal: keyPairInternal,
		Custom:   keyPairCustom,
	}, nil
}

func readKeyPair(certPath, keyPath string) (domain.KeyPair, error) {
	cert, err := os.ReadFile(certPath)
	if err != nil {
		return domain.KeyPair{}, fmt.Errorf("read TLS cert: %w", err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return domain.KeyPair{}, fmt.Errorf("read TLS key: %w", err)
	}

	return domain.KeyPair{Cert: cert, Key: key}, nil
}

func writeKeyPair(keyPair domain.KeyPair, certPath, keyPath string) error {
	if err := os.WriteFile(certPath, keyPair.Cert, 0644); err != nil {
		return fmt.Errorf("write TLS cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPair.Key, 0600); err != nil {
		return fmt.Errorf("write TLS key: %w", err)
	}

	return nil
}

func filesExist(paths ...string) (bool, error) {
	for _, path := range paths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false, nil
		} else if err != nil {
			return false, fmt.Errorf("check file %s: %w", path, err)
		}
	}

	return true, nil
}
