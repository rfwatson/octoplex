package token_test

import (
	"os"
	"path/filepath"
	"testing"

	"git.netflux.io/rob/octoplex/internal/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndCompareToken(t *testing.T) {
	rawToken, hashedToken, err := token.Generate()
	require.NoError(t, err)

	assert.Len(t, rawToken, 64)    // 32 bytes hex-encoded
	assert.Len(t, hashedToken, 76) // scrypt$<salt>$<hash> format

	anotherRawToken, anotherHashedToken, err := token.Generate()
	require.NoError(t, err)

	assert.NotEqual(t, rawToken, anotherRawToken)
	assert.NotEqual(t, hashedToken, anotherHashedToken)

	isEqual, err := token.Compare(rawToken, hashedToken)
	require.NoError(t, err)
	require.True(t, isEqual)

	isEqual, err = token.Compare(anotherRawToken, anotherHashedToken)
	require.NoError(t, err)
	require.True(t, isEqual)

	isEqual, err = token.Compare(rawToken, anotherHashedToken)
	require.NoError(t, err)
	require.False(t, isEqual)

	isEqual, err = token.Compare(anotherRawToken, hashedToken)
	require.NoError(t, err)
	require.False(t, isEqual)
}

func TestWriteToken(t *testing.T) {
	tempDir := t.TempDir()
	rawToken, hashedToken, err := token.Write(tempDir)

	require.NoError(t, err)
	assert.NotEmpty(t, rawToken)
	assert.NotEmpty(t, hashedToken)

	hashedToken2, err := token.Read(tempDir) // should return the same token
	require.NoError(t, err)
	assert.Equal(t, hashedToken, hashedToken2)

	fi, err := os.Stat(filepath.Join(tempDir, "token.txt"))
	require.NoError(t, err)
	assert.Equal(t, fi.Mode().Perm(), os.FileMode(0600))
}
