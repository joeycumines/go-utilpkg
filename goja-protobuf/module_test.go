package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestNew_NilRuntime_Panics(t *testing.T) {
	assert.PanicsWithValue(t, "gojaprotobuf: runtime must not be nil", func() {
		_, _ = New(nil)
	})
}

func TestNew_Default(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)
	assert.NotNil(t, m)
	assert.Equal(t, rt, m.Runtime())
	assert.Equal(t, protoregistry.GlobalTypes, m.resolver)
	assert.Equal(t, protoregistry.GlobalFiles, m.files)
	assert.NotNil(t, m.localTypes)
	assert.NotNil(t, m.localFiles)
}

func TestNew_WithOptions(t *testing.T) {
	rt := goja.New()
	r := new(protoregistry.Types)
	f := new(protoregistry.Files)

	m, err := New(rt, WithResolver(r), WithFiles(f))
	require.NoError(t, err)
	assert.Equal(t, r, m.resolver)
	assert.Equal(t, f, m.files)
}

func TestRuntime_Accessor(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)
	assert.Same(t, rt, m.Runtime())
}
