package mediaserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneratePassword(t *testing.T) {
	assert.Len(t, generatePassword(), 64)
	assert.NotEqual(t, generatePassword(), generatePassword())
}
