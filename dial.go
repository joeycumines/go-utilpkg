package gojagrpc

import (
	"github.com/dop251/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dialConn wraps a [grpc.ClientConn] obtained via [grpc.NewClient].
// It is stored as a native Go object on the JS channel wrapper and
// extracted by [Module.parseChannelOpt] when passed to createClient.
type dialConn struct {
	conn   *grpc.ClientConn
	target string
}

// jsDial implements the JS-facing grpc.dial(target, opts?) function.
// It creates a gRPC client connection to the specified target using
// [grpc.NewClient]. No I/O is performed until the first RPC.
//
// Options:
//   - insecure: bool — use plaintext (no TLS) connection
//   - authority: string — override the :authority header
//
// Returns a JS channel object with:
//   - close() — close the underlying connection
//   - target() — return the dial target string
//
// The returned channel can be passed to createClient via the
// { channel: ch } option.
func (m *Module) jsDial(call goja.FunctionCall) goja.Value {
	target := call.Argument(0).String()
	if target == "" {
		panic(m.runtime.NewTypeError("dial: target must be a non-empty string"))
	}

	var dialOpts []grpc.DialOption

	// Parse options.
	optsArg := call.Argument(1)
	if optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
		if optsObj, ok := optsArg.(*goja.Object); ok {
			// insecure option — plaintext transport.
			insecureVal := optsObj.Get("insecure")
			if insecureVal != nil && !goja.IsUndefined(insecureVal) && insecureVal.ToBoolean() {
				dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
			}

			// authority option — override :authority header.
			authorityVal := optsObj.Get("authority")
			if authorityVal != nil && !goja.IsUndefined(authorityVal) && !goja.IsNull(authorityVal) {
				dialOpts = append(dialOpts, grpc.WithAuthority(authorityVal.String()))
			}
		}
	}

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		panic(m.runtime.NewTypeError("dial: %s", err))
	}

	dc := &dialConn{conn: conn, target: target}

	// Build JS channel wrapper object.
	obj := m.runtime.NewObject()
	_ = obj.Set("close", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if closeErr := dc.conn.Close(); closeErr != nil {
			panic(m.runtime.NewTypeError("close: %s", closeErr))
		}
		return goja.Undefined()
	}))
	_ = obj.Set("target", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return m.runtime.ToValue(dc.target)
	}))
	// Store native connection for createClient extraction.
	_ = obj.Set("_conn", m.runtime.ToValue(dc))

	return obj
}

// parseChannelOpt extracts a [grpc.ClientConnInterface] from the
// createClient options object. If no channel option is present, the
// module's default in-process channel is returned.
//
// Must be called on the event loop goroutine.
func (m *Module) parseChannelOpt(optsObj *goja.Object) grpc.ClientConnInterface {
	val := optsObj.Get("channel")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return m.channel
	}

	chObj, ok := val.(*goja.Object)
	if !ok {
		panic(m.runtime.NewTypeError("channel must be a dial() result"))
	}

	connVal := chObj.Get("_conn")
	if connVal == nil || goja.IsUndefined(connVal) {
		panic(m.runtime.NewTypeError("channel must be a dial() result"))
	}

	dc, ok := connVal.Export().(*dialConn)
	if !ok {
		panic(m.runtime.NewTypeError("channel must be a dial() result"))
	}

	return dc.conn
}
