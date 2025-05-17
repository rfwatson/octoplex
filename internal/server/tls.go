package server

import (
	"fmt"
	"os"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/tls"
)

// buildKeyPairs builds TLS key pairs for the given custom TLS configuration.
//
// It generates a self-signed certificate (valid for localhost, and any
// provided customHost unless a custom key pair is provided) and loads any
// optionally provided custom TLS certs (which must be valid for customHost, if
// provided).
//
// TODO: verify that the custom cert is valid for the custom host
// TODO: persist the generated keypair between runs
func buildKeyPairs(customHost string, customCfg *config.TLS) (_ domain.KeyPairs, err error) {
	dnsNames := []string{"localhost"}
	if customHost != "" && customCfg == nil {
		dnsNames = append(dnsNames, customHost)
	}

	keyPairInternal, err := tls.GenerateKeyPair(dnsNames...)
	if err != nil {
		return domain.KeyPairs{}, fmt.Errorf("generate TLS cert: %w", err)
	}

	var keyPairCustom domain.KeyPair
	if customCfg != nil {
		keyPairCustom.Cert, err = os.ReadFile(customCfg.CertPath)
		if err != nil {
			return domain.KeyPairs{}, fmt.Errorf("read TLS cert: %w", err)
		}
		keyPairCustom.Key, err = os.ReadFile(customCfg.KeyPath)
		if err != nil {
			return domain.KeyPairs{}, fmt.Errorf("read TLS key: %w", err)
		}
	}

	return domain.KeyPairs{
		Internal: keyPairInternal,
		Custom:   keyPairCustom,
	}, nil
}
