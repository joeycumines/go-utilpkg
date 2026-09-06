package inprocgrpc

import (
	"fmt"
	"sync"

	goeventloop "github.com/joeycumines/go-eventloop"
	"google.golang.org/grpc"
)

// Channel is an event-loop-driven in-process gRPC client connection and
// service registrar. The zero value is not usable.
type Channel struct {
	cloner         Cloner
	loop           Loop
	unaryInt       grpc.UnaryServerInterceptor
	streamInt      grpc.StreamServerInterceptor
	clientStats    *statsHandlerHelper
	serverStats    *statsHandlerHelper
	streamHandlers map[string]StreamHandlerFunc
	handlers       handlerMap
	streamBuffer   int
	cloneDisabled  bool
	registrationMu sync.RWMutex
}

// ServiceRegistration describes one generated gRPC service in a
// [RegistrationBatch].
type ServiceRegistration struct {
	Descriptor     *grpc.ServiceDesc
	Implementation any
}

// StreamHandlerRegistration describes one event-loop-native full-method
// handler in a [RegistrationBatch].
type StreamHandlerRegistration struct {
	Method  string
	Handler StreamHandlerFunc
}

// RegistrationBatch is one atomic set of generated services and
// event-loop-native handlers.
type RegistrationBatch struct {
	Services       []ServiceRegistration
	StreamHandlers []StreamHandlerRegistration
}

// UnregistrationBatch is one atomic set of service and event-loop-native
// handler removals. A name that is not currently registered is a silent
// no-op, because the same teardown path serves a failed admission (nothing
// was published) and a real disposal (entries were published). Services are
// identified by service name; stream handlers by full method name.
type UnregistrationBatch struct {
	Services       []string
	StreamHandlers []string
}

// LoopOwner authenticates ownership of one concrete event loop without
// exposing that loop through an inward accessor.
type LoopOwner interface {
	OwnsLoop(*goeventloop.Loop) bool
}

var (
	_ grpc.ClientConnInterface = (*Channel)(nil)
	_ grpc.ServiceRegistrar    = (*Channel)(nil)
)

// NewChannel creates an event-loop-driven in-process gRPC channel. Callers
// must provide a non-nil scheduler through [WithLoop]; it must be running
// before RPCs begin. Messages are isolated with [ProtoCloner] unless another
// cloner or [WithCloneDisabled] is supplied.
//
// NewChannel panics when an option is nil or invalid, or when [WithLoop] is
// omitted. These are static configuration contract violations.
func NewChannel(opts ...ChannelOption) *Channel {
	cfg, err := resolveOptions(opts)
	if err != nil {
		panic(fmt.Sprintf("inprocgrpc: %v", err))
	}
	cloner := Cloner(ProtoCloner{})
	if cfg.cloner != nil {
		cloner = cfg.cloner
	}
	return &Channel{
		loop:          cfg.loop,
		cloner:        cloner,
		cloneDisabled: cfg.cloneDisabled,
		unaryInt:      cfg.unaryInterceptor,
		streamInt:     cfg.streamInterceptor,
		clientStats:   cfg.clientStats,
		serverStats:   cfg.serverStats,
		streamBuffer:  cfg.streamBuffer,
	}
}

// SharesLoop reports whether the channel dispatches on the exact event loop
// authenticated by owner. Channels backed by another Loop implementation
// return false.
func (c *Channel) SharesLoop(owner LoopOwner) bool {
	if c == nil || isNil(owner) {
		return false
	}
	loop, ok := c.loop.(*goeventloop.Loop)
	return ok && loop != nil && owner.OwnsLoop(loop)
}

// RegisterService registers a generated gRPC service implementation.
func (c *Channel) RegisterService(desc *grpc.ServiceDesc, svr any) {
	if err := validateService(desc, svr); err != nil {
		panic(fmt.Sprintf("inprocgrpc: %v", err))
	}
	if err := c.RegisterBatch(RegistrationBatch{Services: []ServiceRegistration{{
		Descriptor:     desc,
		Implementation: svr,
	}}}); err != nil {
		panic(err.Error())
	}
}

// GetServiceInfo returns information about registered services.
func (c *Channel) GetServiceInfo() map[string]grpc.ServiceInfo {
	c.registrationMu.RLock()
	defer c.registrationMu.RUnlock()
	return c.handlers.getServiceInfo()
}

// RegisterStreamHandler registers a non-blocking owner-thread handler for a
// full method name. It takes priority over a generated service handler.
//
// Registration is synchronized with RPC dispatch. A successful registration
// becomes visible to new lookups atomically.
func (c *Channel) RegisterStreamHandler(method string, handler StreamHandlerFunc) {
	if len(method) == 0 || method[0] != '/' {
		panic(fmt.Sprintf(
			"inprocgrpc: method name must start with '/': %q",
			method,
		))
	}
	if handler == nil {
		panic("inprocgrpc: stream handler must not be nil")
	}
	if err := c.RegisterBatch(RegistrationBatch{
		StreamHandlers: []StreamHandlerRegistration{{
			Method:  method,
			Handler: handler,
		}},
	}); err != nil {
		panic(err.Error())
	}
}

// UnregisterStreamHandler removes an event-loop-native handler for a full
// method name. If no handler is registered for the method, it is a silent
// no-op. Unregistration is synchronized with RPC dispatch exactly like
// [Channel.RegisterStreamHandler]: in-flight RPCs that already snapshotted the
// handler continue unaffected, while new lookups observe the removal.
//
// UnregisterStreamHandler panics on a static contract violation, mirroring
// [Channel.RegisterStreamHandler]: a method name not starting with '/' panics
// here with "must start with '/'", while any other malformed shape (e.g. an
// empty service or method segment) panics from [Channel.UnregisterBatch] with
// its full-form message.
func (c *Channel) UnregisterStreamHandler(method string) {
	if len(method) == 0 || method[0] != '/' {
		panic(fmt.Sprintf(
			"inprocgrpc: method name must start with '/': %q",
			method,
		))
	}
	c.UnregisterBatch(UnregistrationBatch{
		StreamHandlers: []string{method},
	})
}

// UnregisterService removes a generated gRPC service registration by service
// name. If the service is not registered, it is a silent no-op. A stream
// handler registered for the same method is NOT removed by this call; remove
// stream handlers explicitly via [Channel.UnregisterStreamHandler] (or use
// [Channel.UnregisterBatch] to remove both atomically).
//
// UnregisterService panics if serviceName is empty: a static contract
// violation.
func (c *Channel) UnregisterService(serviceName string) {
	if serviceName == "" {
		panic("inprocgrpc: service name must not be empty")
	}
	c.UnregisterBatch(UnregistrationBatch{
		Services: []string{serviceName},
	})
}

// RegisterBatch atomically registers generated services and
// event-loop-native stream handlers. RPC dispatch observes either the complete
// batch or none of it.
//
// RegisterBatch panics if the channel is nil or the batch itself is invalid.
// It returns an error if a service or full method conflicts with the channel's
// already-published registry. The complete batch is validated before
// publication, so either failure leaves the registry unchanged.
func (c *Channel) RegisterBatch(batch RegistrationBatch) error {
	if c == nil {
		panic("inprocgrpc: channel must not be nil")
	}
	serviceNames := make(map[string]struct{}, len(batch.Services))
	for index, service := range batch.Services {
		if err := validateService(
			service.Descriptor,
			service.Implementation,
		); err != nil {
			panic(fmt.Errorf(
				"inprocgrpc: service registration %d: %v",
				index,
				err,
			))
		}
		name := service.Descriptor.ServiceName
		if _, exists := serviceNames[name]; exists {
			panic(fmt.Errorf(
				"inprocgrpc: duplicate service %q in registration batch",
				name,
			))
		}
		serviceNames[name] = struct{}{}
	}
	methodNames := make(map[string]struct{}, len(batch.StreamHandlers))
	for index, streamHandler := range batch.StreamHandlers {
		normalized, _, _, methodErr := validateMethod(streamHandler.Method)
		if methodErr != nil || normalized != streamHandler.Method {
			panic(fmt.Errorf(
				"inprocgrpc: stream handler registration %d method must have form '/service/method': %q",
				index,
				streamHandler.Method,
			))
		}
		if streamHandler.Handler == nil {
			panic(fmt.Errorf(
				"inprocgrpc: stream handler registration %d handler must not be nil",
				index,
			))
		}
		if _, exists := methodNames[streamHandler.Method]; exists {
			panic(fmt.Errorf(
				"inprocgrpc: duplicate stream handler %q in registration batch",
				streamHandler.Method,
			))
		}
		methodNames[streamHandler.Method] = struct{}{}
	}

	c.registrationMu.Lock()
	defer c.registrationMu.Unlock()
	c.handlers.mu.Lock()
	defer c.handlers.mu.Unlock()

	for _, service := range batch.Services {
		name := service.Descriptor.ServiceName
		if _, exists := c.handlers.services[name]; exists {
			return fmt.Errorf(
				"inprocgrpc: service %q already registered",
				name,
			)
		}
	}
	for _, streamHandler := range batch.StreamHandlers {
		method := streamHandler.Method
		if _, exists := c.streamHandlers[method]; exists {
			return fmt.Errorf(
				"inprocgrpc: stream handler already registered for %q",
				method,
			)
		}
	}
	if len(batch.Services) != 0 && c.handlers.services == nil {
		c.handlers.services = make(map[string]serviceEntry)
	}
	if len(batch.StreamHandlers) != 0 && c.streamHandlers == nil {
		c.streamHandlers = make(map[string]StreamHandlerFunc)
	}
	for _, service := range batch.Services {
		c.handlers.registerServiceLocked(
			service.Descriptor,
			service.Implementation,
		)
	}
	for _, streamHandler := range batch.StreamHandlers {
		c.streamHandlers[streamHandler.Method] = streamHandler.Handler
	}
	return nil
}

// UnregisterBatch atomically removes generated services and
// event-loop-native stream handlers. RPC dispatch observes the removal on the
// next lookup; RPCs that already snapshotted their target are unaffected (the
// captured callback closure or service entry outlives the map delete).
//
// A service or method that is not currently registered is a silent no-op. This
// idempotence is required because the same teardown path runs for both a
// failed admission (nothing was ever published) and a genuine disposal, and
// those two callers cannot be distinguished at the channel boundary. Removal
// therefore has no runtime failure mode, so UnregisterBatch reports nothing:
// invalid input panics as a static contract violation (mirroring
// [Channel.RegisterBatch]).
//
// UnregisterBatch panics if the channel is nil, a service name is empty, or
// a stream-handler method has an invalid shape. The removal is atomic with
// respect to dispatch because it is published under the same
// registrationMu/handlers.mu lock pair as [Channel.RegisterBatch].
func (c *Channel) UnregisterBatch(batch UnregistrationBatch) {
	if c == nil {
		panic("inprocgrpc: channel must not be nil")
	}
	for index, service := range batch.Services {
		if service == "" {
			panic(fmt.Errorf(
				"inprocgrpc: service removal %d name must not be empty",
				index,
			))
		}
	}
	for index, method := range batch.StreamHandlers {
		normalized, _, _, methodErr := validateMethod(method)
		if methodErr != nil || normalized != method {
			panic(fmt.Errorf(
				"inprocgrpc: stream handler removal %d method must have form '/service/method': %q",
				index,
				method,
			))
		}
	}

	c.registrationMu.Lock()
	defer c.registrationMu.Unlock()
	c.handlers.mu.Lock()
	defer c.handlers.mu.Unlock()

	for _, name := range batch.Services {
		c.handlers.unregisterServiceLocked(name)
	}
	for _, method := range batch.StreamHandlers {
		delete(c.streamHandlers, method)
	}
}
