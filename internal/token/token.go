package token

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	scryptN = 1 << 15 // 32768
	scryptR = 8
	scryptP = 1
	saltLen = 16
)

// RawToken is a type that represents a raw token in hexadecimal format.
//
// It must not be persisted by the server.
type RawToken string

// Generate creates a new random token and returns its raw hexadecimal
// representation and a hashed version suitable for storage.
func Generate(keyLen int) (RawToken, string, error) {
	raw := make([]byte, keyLen)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}

	hash, err := scrypt.Key(raw, salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return "", "", err
	}

	params := fmt.Sprintf("%d,%d,%d,%d", scryptN, scryptR, scryptP, keyLen)
	saltB64 := base64.StdEncoding.EncodeToString(salt)
	hashB64 := base64.StdEncoding.EncodeToString(hash)
	hashed := fmt.Sprintf("scrypt$%s$%s$%s", params, saltB64, hashB64)

	return RawToken(hex.EncodeToString(raw)), hashed, nil
}

// Compare checks if the provided raw token matches the stored token.
func Compare(rawToken RawToken, hashedToken string) (bool, error) {
	parts := strings.Split(hashedToken, "$")
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
	raw, err := hex.DecodeString(string(rawToken))
	if err != nil {
		return false, err
	}

	hash, err := scrypt.Key(raw, salt, n, r, p, keyLen)
	if err != nil {
		return false, err
	}

	return subtle.ConstantTimeCompare(hash, storedHash) == 1, nil
}

// Write generates a new token, writes it to the specified path, and
// returns the raw and stored representations.
func Write(path string, keyLen int) (RawToken, string, error) {
	rawToken, hashedToken, err := Generate(keyLen)
	if err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}

	if err := os.WriteFile(path, []byte(hashedToken), 0600); err != nil {
		return "", "", fmt.Errorf("write file: %w", err)
	}

	return rawToken, hashedToken, nil
}

// ErrTokenNotFound is returned when the token file does not exist.
var ErrTokenNotFound = errors.New("token not found")

// Read reads the stored token from the specified path and returns it.
func Read(path string) (string, error) {
	if hashedToken, err := os.ReadFile(path); errors.Is(err, os.ErrNotExist) {
		return "", ErrTokenNotFound
	} else if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	} else {
		return string(hashedToken), nil
	}
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
