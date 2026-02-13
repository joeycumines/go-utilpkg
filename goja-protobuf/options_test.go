package gojaprotobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestResolveOptions_Empty(t *testing.T) {
	cfg, err := resolveOptions(nil)
	require.NoError(t, err)
	assert.Nil(t, cfg.resolver)
	assert.Nil(t, cfg.files)
}

func TestResolveOptions_NilOption(t *testing.T) {
	cfg, err := resolveOptions([]Option{nil, nil})
	require.NoError(t, err)
	assert.Nil(t, cfg.resolver)
	assert.Nil(t, cfg.files)
}

func TestWithResolver(t *testing.T) {
	r := new(protoregistry.Types)
	cfg, err := resolveOptions([]Option{WithResolver(r)})
	require.NoError(t, err)
	assert.Equal(t, r, cfg.resolver)
	assert.Nil(t, cfg.files)
}

func TestWithFiles(t *testing.T) {
	f := new(protoregistry.Files)
	cfg, err := resolveOptions([]Option{WithFiles(f)})
	require.NoError(t, err)
	assert.Nil(t, cfg.resolver)
	assert.Equal(t, f, cfg.files)
}
