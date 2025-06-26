package optional_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/optional"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	t.Run("string value", func(t *testing.T) {
		opt := optional.New("hello")
		assert.True(t, opt.Present)
		assert.Equal(t, "hello", opt.Value)
	})

	t.Run("int value", func(t *testing.T) {
		opt := optional.New(42)
		assert.True(t, opt.Present)
		assert.Equal(t, 42, opt.Value)
	})

	t.Run("struct value", func(t *testing.T) {
		type testStruct struct {
			Name string
			Age  int
		}
		s := testStruct{Name: "test", Age: 30}
		opt := optional.New(s)
		assert.True(t, opt.Present)
		assert.Equal(t, s, opt.Value)
	})
}

func TestEmpty(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		opt := optional.Empty[string]()
		assert.False(t, opt.Present)
		assert.Equal(t, "", opt.Value)
	})

	t.Run("empty int", func(t *testing.T) {
		opt := optional.Empty[int]()
		assert.False(t, opt.Present)
		assert.Equal(t, 0, opt.Value)
	})

	t.Run("empty pointer", func(t *testing.T) {
		opt := optional.Empty[*string]()
		assert.False(t, opt.Present)
		assert.Nil(t, opt.Value)
	})
}

func TestDeref(t *testing.T) {
	t.Run("non-nil pointer", func(t *testing.T) {
		s := "hello"
		opt := optional.Deref(&s)
		assert.True(t, opt.Present)
		assert.Equal(t, "hello", opt.Value)
	})

	t.Run("nil pointer", func(t *testing.T) {
		opt := optional.Deref[string](nil)
		assert.False(t, opt.Present)
		assert.Equal(t, "", opt.Value)
	})

	t.Run("pointer to int", func(t *testing.T) {
		i := 42
		opt := optional.Deref(&i)
		assert.True(t, opt.Present)
		assert.Equal(t, 42, opt.Value)
	})
}

func TestRef(t *testing.T) {
	t.Run("present value", func(t *testing.T) {
		opt := optional.New("hello")
		ref := opt.Ref()
		assert.NotNil(t, ref)
		assert.Equal(t, "hello", *ref)
	})

	t.Run("not present value", func(t *testing.T) {
		opt := optional.Empty[string]()
		ref := opt.Ref()
		assert.Nil(t, ref)
	})

	t.Run("zero value when present", func(t *testing.T) {
		opt := optional.New(0)
		ref := opt.Ref()
		assert.NotNil(t, ref)
		assert.Equal(t, 0, *ref)
	})
}

func TestIsZero(t *testing.T) {
	t.Run("empty optional", func(t *testing.T) {
		opt := optional.Empty[string]()
		assert.True(t, opt.IsZero())
	})

	t.Run("present optional", func(t *testing.T) {
		opt := optional.New("value")
		assert.False(t, opt.IsZero())
	})

	t.Run("present with zero value", func(t *testing.T) {
		opt := optional.New(0)
		assert.False(t, opt.IsZero())
	})
}

func TestIsPresent(t *testing.T) {
	t.Run("empty optional", func(t *testing.T) {
		opt := optional.Empty[string]()
		assert.False(t, opt.IsPresent())
	})

	t.Run("present optional", func(t *testing.T) {
		opt := optional.New("value")
		assert.True(t, opt.IsPresent())
	})

	t.Run("from non-nil pointer", func(t *testing.T) {
		s := "test"
		opt := optional.Deref(&s)
		assert.True(t, opt.IsPresent())
	})

	t.Run("from nil pointer", func(t *testing.T) {
		opt := optional.Deref[string](nil)
		assert.False(t, opt.IsPresent())
	})
}
