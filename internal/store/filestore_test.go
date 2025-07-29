package store_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"git.netflux.io/rob/octoplex/internal/shortid"
	"git.netflux.io/rob/octoplex/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilestore(t *testing.T) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("octoplex-%s.json", shortid.New().String()))
	assert.NoFileExists(t, path)
	st, err := store.New(path)
	require.NoError(t, err)
	assert.FileExists(t, path)
	t.Cleanup(func() { os.RemoveAll(path) }) //nolint:errcheck

	state := st.Get()
	assert.Zero(t, state)

	_, err = st.Set(store.State{Destinations: []store.Destination{{Name: "test", URL: "rtmp://localhost/live"}}})
	require.NoError(t, err)
	state = st.Get()
	assert.Len(t, state.Destinations, 1)
	assert.Equal(t, "test", state.Destinations[0].Name)
	assert.Equal(t, "rtmp://localhost/live", state.Destinations[0].URL)

	// reload from disk
	st, err = store.New(path)
	require.NoError(t, err)
	state = st.Get()
	assert.Len(t, state.Destinations, 1)
	assert.Equal(t, "test", state.Destinations[0].Name)
	assert.Equal(t, "rtmp://localhost/live", state.Destinations[0].URL)
}

func TestFilestoreValidation(t *testing.T) {
	testCases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{
			name: "valid",
			in:   `{"destinations":[{"name":"test","url":"rtmp://localhost/live"}]}`,
		},
		{
			name:    "empty destination name",
			in:      `{"destinations":[{"name":"","url":"rtmp://localhost/live"}]}`,
			wantErr: "stored state is invalid: [name]",
		},
		{
			name:    "destination name contains only whitespace",
			in:      `{"destinations":[{"name":"   ","url":"rtmp://localhost/live"}]}`,
			wantErr: "stored state is invalid: [name]",
		},
		{
			name:    "invalid destination URL",
			in:      `{"destinations":[{"name":"test","url":"invalid-url"}]}`,
			wantErr: "stored state is invalid: [url]",
		},
		{
			name:    "invalid scheme",
			in:      `{"destinations":[{"name":"test","url":"invalid-scheme://localhost/live"}]}`,
			wantErr: "stored state is invalid: [url]",
		},
		{
			name:    "duplicate destination",
			in:      `{"destinations":[{"name":"test","url":"rtmp://localhost/live"},{"name":"test2","url":"rtmp://localhost/live"}]}`,
			wantErr: "stored state is invalid: [url]",
		},
		{
			name:    "multiple invalid destinations",
			in:      `{"destinations":[{"name":"test","url":"invalid-url"},{"name":"test2","url":"invalid-scheme://localhost/live"}]}`,
			wantErr: "stored state is invalid: [url]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fptr, err := os.CreateTemp("", "octoplex-"+shortid.New().String())
			require.NoError(t, err)
			t.Cleanup(func() { os.RemoveAll(fptr.Name()) }) //nolint:errcheck

			require.NoError(t, os.WriteFile(fptr.Name(), []byte(tc.in), 0644))

			_, err = store.New(fptr.Name())
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
