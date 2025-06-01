package ptr_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	p := ptr.New(42)
	require.NotNil(t, p)
	assert.Equal(t, 42, *p)
}
