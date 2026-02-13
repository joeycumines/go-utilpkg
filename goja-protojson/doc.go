// Package gojaprotojson provides Protocol Buffers JSON
// (proto JSON) encoding and decoding for the Goja JavaScript engine.
//
// It wraps the standard [google.golang.org/protobuf/encoding/protojson]
// package and exposes marshal, unmarshal, and format functions to JS
// code running in [github.com/dop251/goja].
//
// This module operates on JSON strings for wire-format serialization,
// as opposed to [github.com/joeycumines/goja-protobuf]'s toJSON/fromJSON
// which convert between protobuf messages and native JS objects.
//
// # Usage
//
// Use [Require] to create a [github.com/dop251/goja_nodejs/require.ModuleLoader],
// or create a [Module] directly with [New].
//
//	registry := require.NewRegistry()
//	registry.RegisterNativeModule("protojson", gojaprotojson.Require(
//		gojaprotojson.WithProtobuf(pbModule),
//	))
//
// From JavaScript:
//
//	const protojson = require('protojson');
//	const jsonStr = protojson.marshal(msg);
//	const msg = protojson.unmarshal('my.package.MyMessage', jsonStr);
//	const prettyJson = protojson.format(msg);
package gojaprotojson
