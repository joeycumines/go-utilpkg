// Package gojagrpc provides a JavaScript gRPC module for the goja
// runtime. It bridges [gojaprotobuf] message handling with [inprocgrpc]
// channels, enabling JavaScript code to act as both gRPC clients and
// servers within a Go process.
//
// # Overview
//
// The module exposes gRPC functionality through the [goja_nodejs/require]
// module system, making it available to JavaScript code via:
//
//	const grpc = require('grpc');
//
// All RPC types are supported: unary, server-streaming,
// client-streaming, and bidirectional streaming. Client calls return
// promises. Server handlers run through the bound adapter's serialized logical
// callback owner.
//
// # JavaScript API
//
// The require('grpc') export object provides:
//
//	grpc.createClient(serviceName, opts?) — creates a client proxy
//	grpc.createServer()                  — creates a server builder
//	grpc.createReflectionClient()        — creates a reflection client
//	grpc.enableReflection()              — enables the in-process reflection service
//	grpc.dial(target, opts?)             — creates an external client channel
//	grpc.status                          — status codes and error factory
//	grpc.metadata                        — metadata creation utilities
//
// # Client
//
// A client proxy is created by passing the fully-qualified service name:
//
//	const client = grpc.createClient('my.package.MyService');
//
// Each method on the service descriptor becomes a method on the client
// object, using lowerCamelCase naming. The call pattern depends on the
// RPC type:
//
// Unary:
//
//	const response = await client.myMethod(request, opts?);
//
// Server-streaming:
//
//	const stream = await client.listItems(request, opts?);
//	while (true) {
//	    const { value, done } = await stream.recv();
//	    if (done) break;
//	    // value is a protobuf message wrapper
//	}
//
// Client-streaming:
//
//	const call = await client.upload(opts?);
//	await call.send(msg1);
//	await call.send(msg2);
//	await call.closeSend();
//	const response = await call.response;
//
// Bidi-streaming:
//
//	const stream = await client.chat(opts?);
//	await stream.send(msg1);
//	await stream.send(msg2);
//	await stream.closeSend();
//	while (true) {
//	    const { value, done } = await stream.recv();
//	    if (done) break;
//	}
//
// # Call Options
//
// Client methods accept an optional options object:
//
//	await client.myMethod(request, {
//	    signal: abortController.signal,  // AbortSignal for cancellation
//	    metadata: md,                    // outgoing metadata
//	    timeoutMs: 5000,                 // RPC deadline in milliseconds
//	    onHeader: function(md) {},       // callback for response headers
//	    onTrailer: function(md) {},      // callback for response trailers
//	});
//
// The signal field accepts an AbortSignal from the eventloop package.
// When the signal is aborted, the RPC context is cancelled and the
// promise is rejected with a CANCELLED status error.
// Reflection listServices(opts?), describeService(name, opts?), and
// describeType(name, opts?) accept the same signal and timeoutMs options.
//
// Streaming send admission is bounded and never blocks the JavaScript owner.
// Callers should await each send to apply backpressure. Only one recv may be
// outstanding for a stream; a concurrent recv rejects with FAILED_PRECONDITION.
//
// # Client Interceptors
//
// Clients can be created with interceptors that wrap each unary RPC
// call in a connect-es style onion chain:
//
//	const client = grpc.createClient('myService', {
//	    interceptors: [
//	        function addAuth(next) {
//	            return function(req) {
//	                req.header.set('authorization', 'Bearer token');
//	                return next(req).then(function(resp) {
//	                    // inspect response
//	                    return resp;
//	                });
//	            };
//	        }
//	    ]
//	});
//
// Each interceptor factory receives next and returns a wrapper function.
// The req object has method (string), message (protobuf), and header
// (mutable metadata wrapper). Interceptors can modify metadata, inspect
// responses, implement retry logic, or transform responses.
//
// # Server
//
// A server registers handlers for service methods:
//
//	const server = grpc.createServer();
//	server.addService('my.package.MyService', {
//	    myMethod(request, call) {
//	        return responseMessage;  // or a Promise
//	    },
//	    listItems(request, call) {
//	        call.send(item1);
//	        call.send(item2);
//	    },
//	    upload(call) {
//	        return new Promise((resolve) => {
//	            function read() {
//	                call.recv().then(({ value, done }) => {
//	                    if (done) { resolve(response); return; }
//	                    read();
//	                });
//	            }
//	            read();
//	        });
//	    },
//	    chat(call) {
//	        return new Promise((resolve) => {
//	            function read() {
//	                call.recv().then(({ value, done }) => {
//	                    if (done) { resolve(); return; }
//	                    call.send(echo);
//	                    read();
//	                });
//	            }
//	            read();
//	        });
//	    },
//	});
//	server.start();
//
// Unary and client-streaming handlers return the response (or a Promise
// that resolves to the response). Server-streaming and bidi handlers
// use call.send() to write responses and signal completion by returning
// (or resolving a Promise).
//
// Server handlers receive a call object with:
//   - requestHeader: read-only metadata wrapper with incoming headers
//   - setHeader(md): buffer response headers
//   - sendHeader(): flush buffered headers immediately
//   - setTrailer(md): set response trailers
//   - method: (set by interceptors) the full gRPC method name
//   - request: (for unary/server-stream) the request message
//   - send(msg): (streaming types) send a response message
//   - recv(): (client-stream/bidi) receive next client message
//
// # Server Interceptors
//
// Servers support interceptors for cross-cutting concerns like
// authentication, logging, and error mapping:
//
//	const server = grpc.createServer();
//	server.addInterceptor(function authCheck(next) {
//	    return function(call) {
//	        var auth = call.requestHeader.get('authorization');
//	        if (!auth) {
//	            throw grpc.status.createError(16, 'unauthenticated');
//	        }
//	        return next(call);
//	    };
//	});
//
// Server interceptors follow the same connect-es onion pattern as
// client interceptors. The call object passed to interceptors includes
// method, requestHeader, and (for unary/server-stream) request.
// Multiple interceptors are chained right-to-left.
//
// # Status Codes
//
// The grpc.status object exposes all standard gRPC status codes as
// integer constants and a factory for creating gRPC errors:
//
//	grpc.status.OK                  // 0
//	grpc.status.CANCELLED           // 1
//	grpc.status.UNKNOWN             // 2
//	grpc.status.INVALID_ARGUMENT    // 3
//	// ... all 17 standard codes
//
//	const err = grpc.status.createError(
//	    grpc.status.NOT_FOUND, 'item not found');
//	throw err;  // in a server handler
//
// GrpcError objects have name, code, message, and details properties.
// The details property is always an array (empty if no details).
//
// # Error Details
//
// GrpcError objects can carry structured error details (similar to
// google.rpc.Status details). Server handlers can attach protobuf
// messages as details:
//
//	const detail = new SomeMessage();
//	detail.set('field', 'value');
//	throw grpc.status.createError(3, 'bad request', [detail]);
//
// Client code receives the details on the error object:
//
//	client.echo(req).catch(function(err) {
//	    err.code;       // 3
//	    err.details;    // array of wrapped protobuf messages
//	    err.details[0].get('field');  // 'value'
//	});
//
// Details are serialized as google.protobuf.Any in the gRPC status proto,
// enabling interop with standard gRPC error details. Every detail must be a
// canonical wrapper from the configured protobuf runtime; invalid values fail
// instead of being omitted. Private identity is retained in a weak store rather
// than a mutable JavaScript property or a Go map that pins error objects.
//
// # Metadata
//
// The grpc.metadata object provides metadata creation:
//
//	const md = grpc.metadata.create();
//	md.set('key', 'value');
//	md.get('key');     // 'value'
//	md.getAll('key');  // ['value']
//	md.delete('key');
//	md.forEach((value, key) => { ... });
//	md.toObject();     // { key: ['value'] }
//
// Metadata can be passed as a call option for outgoing requests.
// Incoming call.requestHeader values are isolated, read-only wrappers.
//
// # Resource Ownership
//
// [Module.Close] is idempotent. It cancels active client calls, active
// JavaScript server calls, and reflection requests, and closes external
// connections created through grpc.dial.
// Independently supplied Go connections remain caller-owned. The JavaScript
// export grpc.close() invokes the same shutdown operation; event-loop terminal
// cleanup invokes it automatically. Each dial result also provides an
// idempotent close() method. After shutdown, new client, server, reflection,
// dial, and export-installation admissions fail; repeated Close calls remain
// harmless.
//
// # Runtime Ownership
//
// Goja runtimes are not goroutine-safe. This module performs all Goja work
// under the exact bound adapter's serialized logical callback-owner role. The
// underlying event loop may temporarily transfer that role to an isolated
// callback worker, so physical goroutine identity is not part of the contract.
// Initial setup may access Goja directly only while the caller has exclusive
// ownership before the loop starts. After callbacks may execute, external
// callers must use [gojaeventloop.Adapter.Submit] and must never access the
// runtime or Goja values concurrently. [gojaeventloop.Adapter.NewPromise] is
// owner-only; its settler may be called from another goroutine, while the result
// callback and native settlement execute under the logical owner.
//
// # Architecture
//
// JavaScript servers and the default client channel use [inprocgrpc.Channel]. A
// client may instead use the channel returned by grpc.dial for external network
// I/O. In-process handlers execute directly on the channel's event loop, whose
// identity is authenticated against the adapter during construction. Client
// calls use goroutines only for blocking Go transport work; Goja values,
// callbacks, and Promise settlement are created under that same logical owner.
//
// # Usage
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	registry := require.NewRegistry()
//	loop, err := eventloop.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer loop.Close()
//	rt := goja.New()
//	if err := rt.Set("__done", cancel); err != nil {
//	    log.Fatal(err)
//	}
//	adapter, err := gojaeventloop.New(loop, rt)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := adapter.Bind(); err != nil {
//	    log.Fatal(err)
//	}
//	channel := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
//	pbMod, err := gojaprotobuf.New(rt)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	descriptorBytes, err := os.ReadFile("api.protoset")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if _, err := pbMod.LoadDescriptorSetBytes(descriptorBytes); err != nil {
//	    log.Fatal(err)
//	}
//	pbExports := rt.NewObject()
//	if err := pbMod.SetupExports(pbExports); err != nil {
//	    log.Fatal(err)
//	}
//	if err := rt.Set("pb", pbExports); err != nil {
//	    log.Fatal(err)
//	}
//
//	registry.RegisterNativeModule("grpc", gojagrpc.Require(
//	    gojagrpc.WithChannel(channel),
//	    gojagrpc.WithProtobuf(pbMod),
//	    gojagrpc.WithAdapter(adapter),
//	))
//	registry.Enable(rt)
//
//	if err := adapter.Submit(func(owner *goja.Runtime) {
//	    if _, err := owner.RunString(`
//	        const grpc = require('grpc');
//	        if (typeof grpc.createClient !== 'function') {
//	            throw new Error('grpc module installation failed');
//	        }
//	        __done();
//	    `); err != nil {
//	        panic(err)
//	    }
//	}); err != nil {
//	    log.Fatal(err)
//	}
//	if err := loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
//	    log.Fatal(err)
//	}
//
// [goja]: github.com/joeycumines/goja
// [goja_nodejs/require]: github.com/joeycumines/goja_nodejs/require
// [gojaprotobuf]: github.com/joeycumines/goja-protobuf
// [inprocgrpc]: github.com/joeycumines/go-inprocgrpc
// [gojaeventloop.Adapter.Submit]: github.com/joeycumines/goja-eventloop
package gojagrpc
