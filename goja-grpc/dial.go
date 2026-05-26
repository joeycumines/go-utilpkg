package gojagrpc

import (
	"sync"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dialConn wraps a [grpc.ClientConn] obtained via [grpc.NewClient].
// It is stored as a native Go object on the JS channel wrapper and
// extracted by [Module.parseChannelOpt] when passed to createClient.
type dialConn struct {
	module  *Module
	control *dialControl
	rootID  supervisorChildID
}

type dialControl struct {
	err    error
	conn   *grpc.ClientConn
	target string
	done   chan struct{}
	once   sync.Once
}

func (d *dialControl) close() error {
	if d == nil {
		return nil
	}
	d.once.Do(func() {
		defer func() {
			if d.done != nil {
				close(d.done)
			}
		}()
		if d.conn != nil {
			d.err = d.conn.Close()
		}
	})
	return d.err
}

func (d *dialControl) stop(error) { _ = d.close() }

func (d *dialControl) wait() <-chan struct{} {
	if d == nil || d.done == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return d.done
}

func (d *dialControl) result() error {
	if d == nil {
		return nil
	}
	return d.err
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
	m.mustOpen("dial")
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

	control := &dialControl{target: target, done: make(chan struct{})}
	dc := &dialConn{module: m, control: control}
	rootID, err := m.control.reserve(supervisorConnection)
	if err != nil {
		panic(m.runtime.NewTypeError("dial: module is closed"))
	}
	dc.rootID = rootID
	if err := m.ensureOwnerRoot(rootID); err != nil {
		m.control.abandon(rootID)
		panic(m.runtime.NewTypeError("dial: module is closed"))
	}
	published := false
	defer func() {
		if !published {
			m.disposeOwnerRootOwner(rootID, errModuleUnavailable)
			m.control.abandon(rootID)
		}
	}()

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		panic(m.runtime.NewTypeError("dial: %s", err))
	}
	control.conn = conn

	// Build JS channel wrapper object.
	obj := m.runtime.NewObject()
	_ = obj.Set("close", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		m.owner.postDoneMu.Lock()
		delete(m.dialObjects, obj)
		m.owner.postDoneMu.Unlock()
		if closeErr := control.close(); closeErr != nil {
			panic(m.runtime.NewTypeError("close: %s", closeErr))
		}
		m.disposeOwnerRootOwner(rootID, errModuleUnavailable)
		return goja.Undefined()
	}))
	_ = obj.Set("target", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return m.runtime.ToValue(control.target)
	}))
	if err := m.addOwnerRootDisposer(rootID, func(error) {
		m.owner.postDoneMu.Lock()
		delete(m.dialObjects, obj)
		m.owner.postDoneMu.Unlock()
	}); err != nil {
		panic(m.runtime.NewTypeError("dial: module is closed"))
	}

	if err := m.executor.install(rootID, control); err != nil {
		_ = control.close()
		panic(m.runtime.NewTypeError("dial: module is closed"))
	}
	if err := m.control.activate(rootID); err != nil {
		published = true
		_ = control.close()
		m.disposeOwnerRootOwner(rootID, errModuleUnavailable)
		panic(m.runtime.NewTypeError("dial: module is closed"))
	}
	m.activateOwnerRoot(rootID)
	published = true
	m.owner.postDoneMu.Lock()
	terminal := m.ownerTerminalLocked()
	if !terminal {
		m.dialObjects[obj] = dc
	}
	m.owner.postDoneMu.Unlock()
	if terminal {
		// The loop died between admission and publication: never register a
		// dial entry the transfer cannot clean up. The module close
		// triggered by Adapter.Done disposes the root.
		_ = control.close()
		panic(m.runtime.NewTypeError("dial: module is closed"))
	}
	if !m.control.open() {
		m.owner.postDoneMu.Lock()
		delete(m.dialObjects, obj)
		m.owner.postDoneMu.Unlock()
		_ = control.close()
		panic(m.runtime.NewTypeError("dial: module is closed"))
	}

	return obj
}

// parseChannelOpt extracts a [grpc.ClientConnInterface] from the
// createClient options object. If no channel option is present, the
// module's default in-process channel is returned.
//
// Must be called by the current logical adapter callback owner.
func (m *Module) parseChannelOpt(optsObj *goja.Object) grpc.ClientConnInterface {
	val := optsObj.Get("channel")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return m.channel
	}

	chObj, ok := val.(*goja.Object)
	if !ok {
		panic(m.runtime.NewTypeError("channel must be a dial() result"))
	}

	m.owner.postDoneMu.Lock()
	dc, ok := m.dialObjects[chObj]
	m.owner.postDoneMu.Unlock()
	if !ok || dc == nil {
		panic(m.runtime.NewTypeError("channel must be a dial() result"))
	}

	return dc.control.conn
}
