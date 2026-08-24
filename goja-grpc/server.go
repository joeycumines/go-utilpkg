package gojagrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var errServerStarted = errors.New("gojagrpc: server already started")

var errServerTerminalFallback = status.Error(
	codes.Internal,
	"server handler exited during terminal conversion",
)

type jsServer struct {
	m            *Module
	obj          *goja.Object
	services     []serviceRegistration
	interceptors []goja.Callable
	started      bool
}

type serviceRegistration struct {
	descriptor protoreflect.ServiceDescriptor
	handlers   map[string]goja.Value
}

type methodStartRegistration struct {
	name string
	plan *serverMethodPlan
	id   serverMethodID
}

type serviceStartRegistration struct {
	descriptor *grpc.ServiceDesc
	methods    []methodStartRegistration
}

type serverRegistrationAdmission struct {
	control *serverRegistrationControl
	rootID  supervisorChildID
	plans   []serverMethodID
}

type serverRegistrationControl struct {
	done chan struct{}
	once sync.Once
}

func newServerRegistrationControl() *serverRegistrationControl {
	return &serverRegistrationControl{done: make(chan struct{})}
}

func (c *serverRegistrationControl) stop(error) {
	c.once.Do(func() { close(c.done) })
}

func (c *serverRegistrationControl) wait() <-chan struct{} { return c.done }

func (*serverRegistrationControl) result() error { return nil }

// removeServerMethodPlans removes the given server method plans from the owner
// bridge. It takes postDoneMu: the root-disposal disposer may run on the Close
// goroutine while a late JS server operation or an in-flight transport stream
// reads the plan map on another goroutine.
//
// This deliberately does NOT unregister the channel entries. Whole-module Close
// leaves the stream-handler entry in place on purpose so that the
// (now-planless) serverHandler closure returns codes.Unavailable — signalling
// "the module existed but is closed" — rather than codes.Unimplemented. The
// channel teardown required for delete/recreate (where the method must be
// fully removable so a re-registration succeeds) is handled separately by
// [Module.disposeServerRegistration], invoked from the per-server disposal API.
func (m *Module) removeServerMethodPlans(ids []serverMethodID) {
	if len(ids) == 0 {
		return
	}
	m.owner.postDoneMu.Lock()
	defer m.owner.postDoneMu.Unlock()
	for _, id := range ids {
		delete(m.owner.serverPlans, id)
	}
}

// disposeServerRegistration removes the given server method plans AND unregisters
// their stream handlers and service entries from the in-process channel. It is
// the teardown used when a single server registration must be fully retired so
// that the same service/method can be registered again — the delete/recreate
// lifecycle. It must NOT be used on whole-module Close, which intentionally
// keeps the stream-handler entry so disposed plans report codes.Unavailable.
//
// Both the stream handler and the service entry are removed atomically in a
// single channel batch. Both must go: the service entry's MethodDesc carries a
// nil Handler (buildStartPlan registers nil-Handler MethodDescs because the
// stream handler is what actually dispatches), so removing only the stream
// handler would leave lookupUnary to fall through to a nil-Handler service entry
// and panic on the next call.
//
// Plans with an invalid or empty fullMethod (e.g. zero-value test plans, or
// plans from a failed admission where nothing was published) are skipped: the
// channel has no entry for them, and attempting to unregister an empty method
// would panic in validateMethod. channel.UnregisterBatch is idempotent on
// missing entries, so skipping is safe.
//
// In-flight RPCs are safe: the lookup path snapshots the stream-handler
// callback under the channel's registrationMu and releases the read lock before
// dispatching, and serverHandler short-circuits to codes.Unavailable once the
// plan is deleted. No lock inversion is possible: postDoneMu is acquired here,
// then channel.UnregisterBatch acquires registrationMu then handlers.mu, and no
// inprocgrpc path ever acquires postDoneMu.
//
// The returned count is the number of plans this call actually found and
// deleted; plans already removed by a concurrent caller contribute nothing.
func (m *Module) disposeServerRegistration(ids []serverMethodID) int {
	if len(ids) == 0 {
		return 0
	}
	var batch inprocgrpc.UnregistrationBatch
	deleted := 0
	m.owner.postDoneMu.Lock()
	for _, id := range ids {
		plan, ok := m.owner.serverPlans[id]
		if !ok {
			continue
		}
		service, _, valid := splitFullMethod(plan.fullMethod)
		if !valid {
			continue
		}
		batch.StreamHandlers = append(batch.StreamHandlers, plan.fullMethod)
		batch.Services = append(batch.Services, service)
		delete(m.owner.serverPlans, id)
		deleted++
	}
	m.owner.postDoneMu.Unlock()
	if len(batch.StreamHandlers) == 0 && len(batch.Services) == 0 {
		return 0
	}
	m.channel.UnregisterBatch(batch)
	return deleted
}

// splitFullMethod parses a gRPC full method name of the form "/service/Method"
// into its service and method components. It reports ok=false for any shape
// that is not exactly "/service/method" (including empty, missing slashes, or
// a leading slash with an empty service/method). It mirrors the canonical form
// produced by buildStartPlan: fmt.Sprintf("/%s/%s", serviceFullName, methodName).
func splitFullMethod(fullMethod string) (service, method string, ok bool) {
	if len(fullMethod) == 0 || fullMethod[0] != '/' {
		return "", "", false
	}
	body := fullMethod[1:]
	for i := 0; i < len(body); i++ {
		if body[i] == '/' {
			service = body[:i]
			method = body[i+1:]
			if service == "" || method == "" {
				return "", "", false
			}
			// Reject any additional '/' — a valid full method has exactly one.
			for j := range method {
				if method[j] == '/' {
					return "", "", false
				}
			}
			return service, method, true
		}
	}
	return "", "", false
}

// rollbackServerRegistrationOwner synchronously consumes every unpublished
// owner and control obligation created by one compound server admission. It
// runs only under the admission boundary, before the channel can observe the
// registration, so no user-derived disposer is invoked.
//
// The bridge map access is guarded by postDoneMu: the admission callback runs
// on the caller's goroutine, which post-Done may be the runtime goroutine
// while the ownership transfer runs on the Close goroutine.
func (m *Module) rollbackServerRegistrationOwner(
	admission *serverRegistrationAdmission,
) {
	if m == nil || admission == nil || admission.rootID == 0 {
		return
	}
	m.removeServerMethodPlans(admission.plans)
	m.owner.postDoneMu.Lock()
	disposal := m.dispatcher.prepareOwnerRootDisposal(
		admission.rootID,
		false,
	)
	if disposal.root != nil {
		clear(disposal.root.promises)
		clear(disposal.root.callbacks)
		clear(disposal.root.disposers)
		disposal.root.disposers = nil
	}
	m.owner.postDoneMu.Unlock()
	m.dispatcher.finishRootClose(admission.rootID)
	m.control.abandon(admission.rootID)
}

// admitServerRegistration owns the nonreturn-safe rollback for one compound
// registration. The callback may publish the registration only as its final
// successful action.
func (m *Module) admitServerRegistration(
	fn func(*serverRegistrationAdmission) error,
) error {
	if m == nil || fn == nil {
		return errModuleClosed
	}
	return m.control.admit(
		supervisorServerRegistration,
		func(rootID supervisorChildID) (err error) {
			admission := &serverRegistrationAdmission{
				control: newServerRegistrationControl(),
				rootID:  rootID,
			}
			admitted := false
			defer func() {
				if !admitted {
					m.rollbackServerRegistrationOwner(admission)
				}
			}()
			if err = m.ensureOwnerRoot(rootID); err != nil {
				return err
			}
			if err = m.executor.install(rootID, admission.control); err != nil {
				return err
			}
			if err = m.addOwnerRootDisposer(rootID, func(error) {
				m.removeServerMethodPlans(admission.plans)
			}); err != nil {
				return err
			}
			if err = fn(admission); err != nil {
				return err
			}
			admitted = true
			return nil
		},
	)
}

type serverRPC struct {
	ctx         context.Context
	module      *Module
	stream      *inprocgrpc.RPCStream
	control     *serverRPCControl
	rootID      supervisorChildID
	mu          sync.Mutex
	recvPending bool
	recvDone    bool
	recvErr     error
}

// serverRPCControl delegates terminal selection and release to RPCStream.
type serverRPCControl struct {
	stream *inprocgrpc.RPCStream
}

func newServerRPC(
	ctx context.Context,
	module *Module,
	stream *inprocgrpc.RPCStream,
) *serverRPC {
	return &serverRPC{
		ctx:     ctx,
		module:  module,
		stream:  stream,
		control: &serverRPCControl{stream: stream},
	}
}

func (r *serverRPC) register() error {
	id, err := r.module.control.reserve(supervisorServerRPC)
	if err != nil {
		return err
	}
	r.rootID = id
	if err := r.module.ensureOwnerRoot(id); err != nil {
		r.module.control.abandon(id)
		return err
	}
	if err := r.module.executor.install(id, r.control); err != nil {
		r.module.disposeOwnerRootOwner(id, errModuleUnavailable)
		r.module.control.abandon(id)
		return err
	}
	if err := r.module.control.activate(id); err != nil {
		r.control.stop(errModuleUnavailable)
		r.module.disposeOwnerRootOwner(id, errModuleUnavailable)
		return err
	}
	r.module.activateOwnerRoot(id)
	r.armCancellation()
	dispatcher := r.module.dispatcher
	stream := r.stream
	control := r.control
	go func() {
		<-control.wait()
		selected, _ := stream.TerminalResult()
		dispatcher.disposeOwnerRootWorker(id, selected)
	}()
	return nil
}

func (r *serverRPC) armCancellation() {
	stop := context.AfterFunc(r.ctx, func() {
		err := r.ctx.Err()
		if err == nil {
			err = context.Canceled
		}
		r.control.stop(status.FromContextError(err).Err())
	})
	go func() {
		<-r.stream.Done()
		stop()
	}()
}

// finish publishes the transport terminal outcome from the runtime owner.
func (r *serverRPC) finish(err error) {
	r.control.finish(err)
}

func (c *serverRPCControl) finish(err error) {
	if c == nil || c.stream == nil {
		return
	}
	c.stream.Finish(err)
}

func (c *serverRPCControl) stop(err error) {
	if c == nil || c.stream == nil {
		return
	}
	if err == nil {
		err = status.Error(codes.Canceled, "RPC aborted")
	}
	c.stream.Abort(err)
}

func (c *serverRPCControl) wait() <-chan struct{} {
	if c == nil || c.stream == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return c.stream.Done()
}

func (c *serverRPCControl) result() error {
	if c == nil || c.stream == nil {
		return nil
	}
	err, _ := c.stream.TerminalResult()
	return err
}

func (r *serverRPC) send(message proto.Message) error {
	if err, selected := r.stream.TerminalResult(); selected {
		if err != nil {
			return err
		}
		return io.EOF
	}
	return r.stream.Send().Send(message)
}

// run atomically admits one owner callback, then converts native panic and
// runtime.Goexit into one Internal terminal attempt. JavaScript exceptions
// normally arrive as callable errors, but a recovered exception is preserved as
// a gRPC status as well.
func (r *serverRPC) run(fn func()) {
	if r == nil || r.stream == nil {
		return
	}
	turn, admitted := r.stream.AdmitCallback()
	if !admitted {
		return
	}
	turn.Run(func() {
		r.runAdmitted(fn)
	})
}

// schedule reserves asynchronous owner ingress through the transport. Promise
// reactions run after the direct handler turn has settled, so they must not
// manufacture direct callback admission.
func (r *serverRPC) schedule(fn func()) {
	if r == nil || r.stream == nil {
		return
	}
	_ = r.stream.ScheduleCallback(func() {
		r.runAdmitted(fn)
	})
}

func (r *serverRPC) runAdmitted(fn func()) {
	returned := false
	finished := false
	defer func() {
		_ = recover()
		if !returned && !finished {
			r.finish(errServerTerminalFallback)
		}
	}()
	defer func() {
		if returned {
			return
		}
		reason := recover()
		if reason == nil {
			r.finish(status.Error(codes.Internal, "handler exited without returning"))
			finished = true
			return
		}
		if err, ok := reason.(error); ok {
			r.finish(r.module.jsErrorToGRPC(err))
			finished = true
			return
		}
		r.finish(status.Errorf(codes.Internal, "handler panic: %v", reason))
		finished = true
	}()
	fn()
	returned = true
}

func (m *Module) jsCreateServer(goja.FunctionCall) goja.Value {
	m.mustOpen("createServer")
	server := &jsServer{m: m}
	object := m.runtime.NewObject()
	_ = object.Set("addService", m.runtime.ToValue(server.addService))
	_ = object.Set("addInterceptor", m.runtime.ToValue(server.addInterceptor))
	_ = object.Set("start", m.runtime.ToValue(server.start))
	server.obj = object
	return object
}

func (s *jsServer) stateErrorLocked() error {
	if !s.m.control.open() {
		return errModuleClosed
	}
	if s.started {
		return errServerStarted
	}
	return nil
}

func (s *jsServer) mustConfigure(operation string) {
	s.m.mu.Lock()
	err := s.stateErrorLocked()
	s.m.mu.Unlock()
	if err != nil {
		panic(s.m.runtime.NewTypeError("%s: %s", operation, err))
	}
}

func (s *jsServer) addInterceptor(call goja.FunctionCall) goja.Value {
	s.mustConfigure("server.addInterceptor")
	fn, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		panic(s.m.runtime.NewTypeError("addInterceptor: argument must be a function"))
	}
	s.m.mu.Lock()
	err := s.stateErrorLocked()
	if err == nil {
		s.interceptors = append(s.interceptors, fn)
	}
	s.m.mu.Unlock()
	if err != nil {
		panic(s.m.runtime.NewTypeError("server.addInterceptor: %s", err))
	}
	return s.obj
}

func (s *jsServer) addService(call goja.FunctionCall) goja.Value {
	s.mustConfigure("server.addService")
	name := call.Argument(0).String()
	descriptor, err := s.m.resolveService(name)
	if err != nil {
		panic(s.m.runtime.NewTypeError(err.Error()))
	}
	object, ok := call.Argument(1).(*goja.Object)
	if !ok {
		panic(s.m.runtime.NewTypeError("addService: handlers must be an object"))
	}
	handlers := make(map[string]goja.Value, descriptor.Methods().Len())
	for index := 0; index < descriptor.Methods().Len(); index++ {
		method := descriptor.Methods().Get(index)
		jsName := lowerFirst(string(method.Name()))
		handler := object.Get(jsName)
		if handler == nil || goja.IsUndefined(handler) {
			panic(s.m.runtime.NewTypeError("addService: missing handler for %q", jsName))
		}
		if _, ok := goja.AssertFunction(handler); !ok {
			panic(s.m.runtime.NewTypeError("addService: handler for %q is not a function", jsName))
		}
		handlers[jsName] = handler
	}
	s.m.mu.Lock()
	err = s.stateErrorLocked()
	if err == nil {
		for _, service := range s.services {
			if service.descriptor.FullName() == descriptor.FullName() {
				err = fmt.Errorf(
					"service %q already added",
					descriptor.FullName(),
				)
				break
			}
		}
	}
	if err == nil {
		s.services = append(s.services, serviceRegistration{
			descriptor: descriptor,
			handlers:   handlers,
		})
	}
	s.m.mu.Unlock()
	if err != nil {
		panic(s.m.runtime.NewTypeError("server.addService: %s", err))
	}
	return s.obj
}

func (s *jsServer) start(goja.FunctionCall) goja.Value {
	s.mustConfigure("server.start")
	plan := s.buildStartPlan()
	err := s.m.admitServerRegistration(
		func(admission *serverRegistrationAdmission) (err error) {
			batch := inprocgrpc.RegistrationBatch{}
			for _, service := range plan {
				batch.Services = append(
					batch.Services,
					inprocgrpc.ServiceRegistration{
						Descriptor:     service.descriptor,
						Implementation: struct{}{},
					},
				)
				for index := range service.methods {
					method := &service.methods[index]
					method.plan.rootID = admission.rootID
					method.id, err = s.m.allocateServerMethodPlan(method.plan)
					if err != nil {
						return err
					}
					admission.plans = append(admission.plans, method.id)
					batch.StreamHandlers = append(
						batch.StreamHandlers,
						inprocgrpc.StreamHandlerRegistration{
							Method:  method.name,
							Handler: s.m.dispatcher.serverHandler(method.id),
						},
					)
				}
			}
			s.m.mu.Lock()
			defer s.m.mu.Unlock()
			if err = s.stateErrorLocked(); err != nil {
				return err
			}
			if err = s.m.control.activate(admission.rootID); err != nil {
				return err
			}
			if err = s.m.channel.RegisterBatch(batch); err != nil {
				return err
			}
			s.m.activateOwnerRoot(admission.rootID)
			s.started = true
			s.services = nil
			s.interceptors = nil
			return nil
		},
	)
	if err != nil {
		panic(s.m.runtime.NewTypeError("server.start: %s", err))
	}
	return goja.Undefined()
}

func (s *jsServer) buildStartPlan() []serviceStartRegistration {
	plan := make([]serviceStartRegistration, 0, len(s.services))
	for _, registration := range s.services {
		serviceDescriptor := &grpc.ServiceDesc{ServiceName: string(registration.descriptor.FullName())}
		servicePlan := serviceStartRegistration{descriptor: serviceDescriptor}
		methods := registration.descriptor.Methods()
		for index := 0; index < methods.Len(); index++ {
			method := methods.Get(index)
			name := lowerFirst(string(method.Name()))
			fullMethod := fmt.Sprintf("/%s/%s", registration.descriptor.FullName(), method.Name())
			handler, _ := goja.AssertFunction(registration.handlers[name])
			kind := serverMethodUnary
			switch {
			case !method.IsStreamingClient() && !method.IsStreamingServer():
				serviceDescriptor.Methods = append(serviceDescriptor.Methods, grpc.MethodDesc{MethodName: string(method.Name())})
			case !method.IsStreamingClient() && method.IsStreamingServer():
				kind = serverMethodServerStream
				serviceDescriptor.Streams = append(serviceDescriptor.Streams, grpc.StreamDesc{
					StreamName: string(method.Name()), ServerStreams: true,
				})
			case method.IsStreamingClient() && !method.IsStreamingServer():
				kind = serverMethodClientStream
				serviceDescriptor.Streams = append(serviceDescriptor.Streams, grpc.StreamDesc{
					StreamName: string(method.Name()), ClientStreams: true,
				})
			default:
				kind = serverMethodBidiStream
				serviceDescriptor.Streams = append(serviceDescriptor.Streams, grpc.StreamDesc{
					StreamName: string(method.Name()), ClientStreams: true, ServerStreams: true,
				})
			}
			servicePlan.methods = append(
				servicePlan.methods,
				methodStartRegistration{
					name: fullMethod,
					plan: &serverMethodPlan{
						module:       s.m,
						fullMethod:   fullMethod,
						method:       method,
						handler:      handler,
						interceptors: append([]goja.Callable(nil), s.interceptors...),
						kind:         kind,
					},
				},
			)
		}
		plan = append(plan, servicePlan)
	}
	return plan
}
