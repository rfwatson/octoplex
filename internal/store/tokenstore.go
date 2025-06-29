package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
)

var ErrTokenNotFound = errors.New("token not found")

// TokenStore is a store which persistently stores tokens in the filesystem.
//
// It will probably be replaced with sqlite3 at some point. All methods are
// safe for use from multiple goroutines.
type TokenStore struct {
	dataDir string
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewTokenStore creates a new TokenStore instance with the specified data directory and logger.
func NewTokenStore(dataDir string, logger *slog.Logger) *TokenStore {
	return &TokenStore{
		dataDir: dataDir,
		logger:  logger,
	}
}

type tokenRecord struct {
	HashedToken string    `json:"hashed_token"`
	ExpiresAt   time.Time `json:"expires_at,omitzero"`
}

// Get retrieves a value from the store by its key.
func (s *TokenStore) Get(key string) (domain.Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fptr, err := os.Open(filepath.Join(s.dataDir, key+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Token{}, ErrTokenNotFound
		}
		return domain.Token{}, fmt.Errorf("open file %s: %w", key, err)
	}

	var record tokenRecord
	if err := json.NewDecoder(fptr).Decode(&record); err != nil {
		return domain.Token{}, fmt.Errorf("unmarshal: %w", err)
	}

	return domain.Token{
		Hashed:    record.HashedToken,
		ExpiresAt: record.ExpiresAt,
	}, nil
}

// Put stores a value in the store under the specified key.
func (s *TokenStore) Put(key string, token domain.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := json.Marshal(tokenRecord{HashedToken: token.Hashed, ExpiresAt: token.ExpiresAt})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err := os.WriteFile(filepath.Join(s.dataDir, key+".json"), record, 0600); err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	return nil
}

// Delete removes a value from the store by its key.
func (s *TokenStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.RemoveAll(filepath.Join(s.dataDir, key+".json")); err != nil {
		return fmt.Errorf("remove file %s: %w", key, err)
	}

	return nil
}
