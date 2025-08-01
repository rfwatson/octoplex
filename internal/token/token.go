// Package token provides functionality to generate, hash, and compare
// cryptographically secure tokens.
package token

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
	"golang.org/x/crypto/scrypt"
)

// scrypt parameters for hashing tokens.
const (
	scryptN = 1 << 15 // 32768
	scryptR = 8
	scryptP = 1
	saltLen = 16
)

var (
	// ErrTokenExpired is returned when the token has expired.
	ErrTokenExpired = errors.New("token expired")
)

// RawToken represents a raw token as a byte slice.
type RawToken []byte

// GenerateRawToken generates a random token of the specified byte length.
func GenerateRawToken(keyLen int) (RawToken, error) {
	raw := make([]byte, keyLen)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}

	return raw, nil
}

// New creates a new TokenRecord by hashing the raw token using scrypt,
// and returning the TokenRecord.
func New(rawToken RawToken, expiresAt time.Time) (domain.Token, error) {
	if len(rawToken) < 6 {
		return domain.Token{}, errors.New("raw token must be at least 6 bytes long")
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return domain.Token{}, err
	}

	hash, err := scrypt.Key(rawToken, salt, scryptN, scryptR, scryptP, len(rawToken))
	if err != nil {
		return domain.Token{}, err
	}

	params := fmt.Sprintf("%d,%d,%d,%d", scryptN, scryptR, scryptP, len(rawToken))
	saltB64 := base64.StdEncoding.EncodeToString(salt)
	hashB64 := base64.StdEncoding.EncodeToString(hash)
	hashed := fmt.Sprintf("scrypt$%s$%s$%s", params, saltB64, hashB64)

	return domain.Token{Hashed: hashed, ExpiresAt: expiresAt}, nil
}

// Matches checks if the given raw token matches the hashed token in the
// TokenRecord.
func Matches(token domain.Token, rawToken RawToken) (bool, error) {
	if len(rawToken) == 0 {
		return false, nil
	}

	if !token.ExpiresAt.IsZero() && time.Now().After(token.ExpiresAt) {
		return false, ErrTokenExpired
	}

	parts := strings.Split(token.Hashed, "$")
	if len(parts) != 4 || parts[0] != "scrypt" {
		return false, errors.New("invalid token format")
	}

	params, saltB64, hashB64 := parts[1], parts[2], parts[3]

	n, r, p, keyLen, err := parseParams(params)
	if err != nil {
		return false, fmt.Errorf("parse params: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return false, err
	}
	storedHash, err := base64.StdEncoding.DecodeString(hashB64)
	if err != nil {
		return false, err
	}

	hash, err := scrypt.Key(rawToken, salt, n, r, p, keyLen)
	if err != nil {
		return false, err
	}

	return subtle.ConstantTimeCompare(hash, storedHash) == 1, nil
}

func parseParams(params string) (int, int, int, int, error) {
	parts := strings.Split(params, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, errors.New("invalid token params format")
	}

	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid scrypt N: %w", err)
	}

	r, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid scrypt R: %w", err)
	}

	p, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid scrypt P: %w", err)
	}

	keyLen, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid scrypt key length: %w", err)
	}

	return n, r, p, keyLen, nil
}
