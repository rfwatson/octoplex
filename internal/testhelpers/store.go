package testhelpers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"git.netflux.io/rob/octoplex/internal/shortid"
	"git.netflux.io/rob/octoplex/internal/store"
	"github.com/stretchr/testify/require"
)

// NewTestStore creates a new file system store, isolated in a temporary file,
// for testing.
func NewTestStore(t *testing.T, state ...store.State) *store.FileStore {
	path := filepath.Join(t.TempDir(), fmt.Sprintf("test-config-%s.json", shortid.New().String()))

	if len(state) > 0 {
		bytes, err := json.Marshal(state[0])
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, bytes, 0644))
	}

	store, err := store.New(store.StaticPath(path))
	require.NoError(t, err)

	t.Cleanup(func() { os.RemoveAll(path) })

	return store
}
