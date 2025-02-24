package shortid_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/shortid"
	"github.com/stretchr/testify/assert"
)

func TestShortID(t *testing.T) {
	id := shortid.New()
	assert.Len(t, id, 6)
	assert.Len(t, id.String(), 12)

	anotherID := shortid.New()
	assert.NotEqual(t, id, anotherID)
}
