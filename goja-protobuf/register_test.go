package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	gojarequire "github.com/dop251/goja_nodejs/require"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequire_LoadsProtobufModule(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", Require())
	registry.Enable(rt)

	v, err := rt.RunString(`
		var pb = require('protobuf');
		typeof pb.loadDescriptorSet === 'function' &&
		typeof pb.loadFileDescriptorProto === 'function' &&
		typeof pb.messageType === 'function' &&
		typeof pb.enumType === 'function' &&
		typeof pb.encode === 'function' &&
		typeof pb.decode === 'function' &&
		typeof pb.toJSON === 'function' &&
		typeof pb.fromJSON === 'function'
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

func TestRequire_CustomModuleName(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("my-pb", Require())
	registry.Enable(rt)

	v, err := rt.RunString(`
		var pb = require('my-pb');
		typeof pb.messageType === 'function'
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}
