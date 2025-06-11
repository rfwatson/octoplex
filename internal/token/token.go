package token

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	scryptN = 1 << 15 // 32768
	scryptR = 8
	scryptP = 1
	saltLen = 16
	keyLen  = 32
)

// RawToken is a type that represents a raw token in hexadecimal format.
//
// It must not be persisted by the server.
type RawToken string

// Generate creates a new random token and returns its raw hexadecimal
// representation and a hashed version suitable for storage.
func Generate() (RawToken, string, error) {
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

	saltB64 := base64.StdEncoding.EncodeToString(salt)
	hashB64 := base64.StdEncoding.EncodeToString(hash)
	hashed := fmt.Sprintf("scrypt$%s$%s", saltB64, hashB64)

	return RawToken(hex.EncodeToString(raw)), hashed, nil
}

// Compare checks if the provided raw token matches the stored token.
func Compare(rawToken RawToken, hashedToken string) (bool, error) {
	parts := strings.Split(hashedToken, "$")
	if len(parts) != 3 || parts[0] != "scrypt" {
		return false, errors.New("invalid token format")
	}

	saltB64, hashB64 := parts[1], parts[2]

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

	hash, err := scrypt.Key(raw, salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return false, err
	}

	return subtle.ConstantTimeCompare(hash, storedHash) == 1, nil
}

// Write generates a new token, writes it to the specified path, and
// returns the raw and stored representations.
func Write(dataDir string) (RawToken, string, error) {
	rawToken, hashedToken, err := Generate()
	if err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}

	if err := os.WriteFile(tokenPath(dataDir), []byte(hashedToken), 0600); err != nil {
		return "", "", fmt.Errorf("write file: %w", err)
	}

	return rawToken, hashedToken, nil
}

// ErrTokenNotFound is returned when the token file does not exist.
var ErrTokenNotFound = errors.New("token not found")

// Read reads the stored token from the specified path and returns it.
func Read(dataDir string) (string, error) {
	if hashedToken, err := os.ReadFile(tokenPath(dataDir)); errors.Is(err, os.ErrNotExist) {
		return "", ErrTokenNotFound
	} else if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	} else {
		return string(hashedToken), nil
	}
}

func tokenPath(dataDir string) string {
	const tokenFileName = "token.txt"
	return filepath.Join(dataDir, tokenFileName)
}
