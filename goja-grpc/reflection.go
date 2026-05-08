package gojagrpc

import (
	"context"
	"fmt"
	"strconv"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// jsCreateReflectionClient implements grpc.createReflectionClient().
// It takes no arguments — the channel is implicit from the module.
// Returns a JS object with listServices(), describeService(name),
// and describeType(name) methods, each returning Promises.
func (m *Module) jsCreateReflectionClient(call goja.FunctionCall) goja.Value {
	m.mustOpen("createReflectionClient")
	obj := m.runtime.NewObject()
	_ = obj.Set("listServices", m.runtime.ToValue(m.jsReflListServices))
	_ = obj.Set("describeService", m.runtime.ToValue(m.jsReflDescribeService))
	_ = obj.Set("describeType", m.runtime.ToValue(m.jsReflDescribeType))
	return obj
}

// EnableReflection registers the gRPC reflection service on the
// underlying channel, enabling JS clients to discover services
// and types at runtime. It uses the protobuf module's shared file and type
// resolvers so dynamically loaded descriptors and extensions retain the
// runtime's protobuf identity and remain visible to reflection.
//
// This should be called after all services have been registered. Repeated calls
// are harmless; calls after module shutdown return an error.
func (m *Module) EnableReflection() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.control.open() {
		return errModuleClosed
	}
	if m.reflectionSet {
		return nil
	}
	reflServer := reflection.NewServerV1(reflection.ServerOptions{
		Services:           m.channel,
		DescriptorResolver: m.protobuf.FileResolver(),
		ExtensionResolver:  m.protobuf.TypeResolver(),
	})
	if err := m.channel.RegisterBatch(inprocgrpc.RegistrationBatch{
		Services: []inprocgrpc.ServiceRegistration{{
			Descriptor:     &reflectionpb.ServerReflection_ServiceDesc,
			Implementation: reflServer,
		}},
	}); err != nil {
		return fmt.Errorf("gojagrpc: enable reflection: %w", err)
	}
	m.reflectionSet = true
	return nil
}

// jsReflListServices returns a Promise<string[]> of all registered service names.
func (m *Module) jsReflListServices(call goja.FunctionCall) goja.Value {
	options := m.parseCallOpts(call, 0)
	promise := m.newOwnerPromise(options.rootID, func(result ownerResult) any {
		services := result.(ownerStringsResult).values
		arr := m.runtime.NewArray()
		for i, service := range services {
			_ = arr.Set(intStr(i), m.runtime.ToValue(service))
		}
		return arr
	}, nil)
	if !promise.admitted() {
		options.finishOwner()
		return promise.value
	}
	if err := options.register(); err != nil {
		_ = m.rejectOwnerPromiseInline(promise.id, err)
		options.finishOwner()
		return promise.value
	}
	if err := options.control.bindRelease(nil); err != nil {
		_ = m.rejectOwnerPromiseInline(promise.id, err)
		options.finishOwner()
		return promise.value
	}

	promiseID := promise.id
	root := options.workerRoot()
	channel := m.channel
	go func() {
		boundary := rootWorkerBoundary{
			root:           root,
			promise:        promiseID,
			transportBound: true,
		}
		boundary.run(func() {
			services, err := doListServicesContext(root.control.ctx, channel)
			var settleErr error
			if err != nil {
				settleErr = root.owner.rejectOwnerPromise(promiseID, err)
			} else {
				settleErr = root.owner.resolveOwnerPromise(promiseID, ownerStringsResult{
					values: append([]string(nil), services...),
				})
			}
			if settleErr != nil {
				root.finish(settleErr)
				return
			}
			root.finish(err)
		})
	}()

	return promise.value
}

// jsReflDescribeService returns a Promise<{name, methods: [...]}>
// for a given fully-qualified service name.
func (m *Module) jsReflDescribeService(call goja.FunctionCall) goja.Value {
	name := call.Argument(0).String()
	options := m.parseCallOpts(call, 1)
	promise := m.newOwnerPromise(options.rootID, func(result ownerResult) any {
		return m.serviceDescriptionValue(result.(ownerServiceResult).value)
	}, nil)
	if !promise.admitted() {
		options.finishOwner()
		return promise.value
	}
	if err := options.register(); err != nil {
		_ = m.rejectOwnerPromiseInline(promise.id, err)
		options.finishOwner()
		return promise.value
	}
	if err := options.control.bindRelease(nil); err != nil {
		_ = m.rejectOwnerPromiseInline(promise.id, err)
		options.finishOwner()
		return promise.value
	}

	promiseID := promise.id
	root := options.workerRoot()
	channel := m.channel
	go func() {
		boundary := rootWorkerBoundary{
			root:           root,
			promise:        promiseID,
			transportBound: true,
		}
		boundary.run(func() {
			desc, err := doDescribeServiceContext(root.control.ctx, channel, name)
			var settleErr error
			if err != nil {
				settleErr = root.owner.rejectOwnerPromise(promiseID, err)
			} else {
				settleErr = root.owner.resolveOwnerPromise(promiseID, ownerServiceResult{value: desc})
			}
			if settleErr != nil {
				root.finish(settleErr)
				return
			}
			root.finish(err)
		})
	}()

	return promise.value
}

// jsReflDescribeType returns a Promise<{name, fields: [...]}>
// for a given fully-qualified message type name.
func (m *Module) jsReflDescribeType(call goja.FunctionCall) goja.Value {
	name := call.Argument(0).String()
	options := m.parseCallOpts(call, 1)
	promise := m.newOwnerPromise(options.rootID, func(result ownerResult) any {
		return m.messageDescriptionValue(result.(ownerTypeResult).value)
	}, nil)
	if !promise.admitted() {
		options.finishOwner()
		return promise.value
	}
	if err := options.register(); err != nil {
		_ = m.rejectOwnerPromiseInline(promise.id, err)
		options.finishOwner()
		return promise.value
	}
	if err := options.control.bindRelease(nil); err != nil {
		_ = m.rejectOwnerPromiseInline(promise.id, err)
		options.finishOwner()
		return promise.value
	}

	promiseID := promise.id
	root := options.workerRoot()
	channel := m.channel
	go func() {
		boundary := rootWorkerBoundary{
			root:           root,
			promise:        promiseID,
			transportBound: true,
		}
		boundary.run(func() {
			desc, err := doDescribeTypeContext(root.control.ctx, channel, name)
			var settleErr error
			if err != nil {
				settleErr = root.owner.rejectOwnerPromise(promiseID, err)
			} else {
				settleErr = root.owner.resolveOwnerPromise(promiseID, ownerTypeResult{value: desc})
			}
			if settleErr != nil {
				root.finish(settleErr)
				return
			}
			root.finish(err)
		})
	}()

	return promise.value
}

// doListServices uses the Go reflection client to list all services.
func (m *Module) doListServices() ([]string, error) {
	return m.doListServicesContext(m.ctx)
}

func (m *Module) doListServicesContext(ctx context.Context) ([]string, error) {
	return doListServicesContext(ctx, m.channel)
}

func doListServicesContext(
	ctx context.Context,
	channel grpc.ClientConnInterface,
) ([]string, error) {
	client := reflectionpb.NewServerReflectionClient(channel)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.CloseSend()

	err = stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{
			ListServices: "",
		},
	})
	if err != nil {
		return nil, err
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	if err := reflectionResponseError(resp); err != nil {
		return nil, err
	}

	listResp := resp.GetListServicesResponse()
	if listResp == nil {
		return nil, errReflectionFailed("unexpected response type")
	}

	services := make([]string, 0, len(listResp.Service))
	for _, s := range listResp.Service {
		services = append(services, s.Name)
	}
	return services, nil
}

// doDescribeService retrieves full service metadata via reflection.
type reflectedMethod struct {
	name            string
	fullName        string
	inputType       string
	outputType      string
	clientStreaming bool
	serverStreaming bool
}

type reflectedService struct {
	name    string
	methods []reflectedMethod
}

func (m *Module) doDescribeService(name string) (*reflectedService, error) {
	return m.doDescribeServiceContext(m.ctx, name)
}

func (m *Module) doDescribeServiceContext(ctx context.Context, name string) (*reflectedService, error) {
	return doDescribeServiceContext(ctx, m.channel, name)
}

func doDescribeServiceContext(
	ctx context.Context,
	channel grpc.ClientConnInterface,
	name string,
) (*reflectedService, error) {
	fd, err := fetchFileDescriptorForSymbolContext(ctx, channel, name)
	if err != nil {
		return nil, err
	}

	// Find the service in the file descriptor.
	files, err := protodesc.NewFiles(fd)
	if err != nil {
		return nil, err
	}

	desc, err := files.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		return nil, err
	}

	svcDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, errReflectionFailed(name + " is not a service")
	}

	result := &reflectedService{
		name:    string(svcDesc.FullName()),
		methods: make([]reflectedMethod, 0, svcDesc.Methods().Len()),
	}
	for i := 0; i < svcDesc.Methods().Len(); i++ {
		md := svcDesc.Methods().Get(i)
		result.methods = append(result.methods, reflectedMethod{
			name:            string(md.Name()),
			fullName:        string(md.FullName()),
			inputType:       string(md.Input().FullName()),
			outputType:      string(md.Output().FullName()),
			clientStreaming: md.IsStreamingClient(),
			serverStreaming: md.IsStreamingServer(),
		})
	}
	return result, nil
}

func (m *Module) serviceDescriptionValue(description *reflectedService) goja.Value {
	obj := m.runtime.NewObject()
	_ = obj.Set("name", description.name)
	methods := m.runtime.NewArray()
	for i, method := range description.methods {
		methodObject := m.runtime.NewObject()
		_ = methodObject.Set("name", method.name)
		_ = methodObject.Set("fullName", method.fullName)
		_ = methodObject.Set("inputType", method.inputType)
		_ = methodObject.Set("outputType", method.outputType)
		_ = methodObject.Set("clientStreaming", method.clientStreaming)
		_ = methodObject.Set("serverStreaming", method.serverStreaming)
		_ = methods.Set(intStr(i), methodObject)
	}
	_ = obj.Set("methods", methods)
	return obj
}

// doDescribeType retrieves message type metadata via reflection.
type reflectedField struct {
	name         string
	messageType  string
	enumType     string
	defaultValue string
	kind         string
	number       int
	repeated     bool
	mapField     bool
	hasDefault   bool
}

type reflectedOneof struct {
	name   string
	fields []string
}

type reflectedMessage struct {
	name   string
	fields []reflectedField
	oneofs []reflectedOneof
}

func (m *Module) doDescribeType(name string) (*reflectedMessage, error) {
	return m.doDescribeTypeContext(m.ctx, name)
}

func (m *Module) doDescribeTypeContext(ctx context.Context, name string) (*reflectedMessage, error) {
	return doDescribeTypeContext(ctx, m.channel, name)
}

func doDescribeTypeContext(
	ctx context.Context,
	channel grpc.ClientConnInterface,
	name string,
) (*reflectedMessage, error) {
	fd, err := fetchFileDescriptorForSymbolContext(ctx, channel, name)
	if err != nil {
		return nil, err
	}

	files, err := protodesc.NewFiles(fd)
	if err != nil {
		return nil, err
	}

	desc, err := files.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		return nil, err
	}

	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, errReflectionFailed(name + " is not a message type")
	}

	result := &reflectedMessage{
		name:   string(msgDesc.FullName()),
		fields: make([]reflectedField, 0, msgDesc.Fields().Len()),
		oneofs: make([]reflectedOneof, 0, msgDesc.Oneofs().Len()),
	}
	for i := 0; i < msgDesc.Fields().Len(); i++ {
		fieldDescriptor := msgDesc.Fields().Get(i)
		field := reflectedField{
			name:       string(fieldDescriptor.Name()),
			number:     int(fieldDescriptor.Number()),
			kind:       fieldDescriptor.Kind().String(),
			repeated:   fieldDescriptor.IsList(),
			mapField:   fieldDescriptor.IsMap(),
			hasDefault: fieldDescriptor.HasDefault(),
		}
		if fieldDescriptor.Kind() == protoreflect.MessageKind || fieldDescriptor.Kind() == protoreflect.GroupKind {
			field.messageType = string(fieldDescriptor.Message().FullName())
		}
		if fieldDescriptor.Kind() == protoreflect.EnumKind {
			field.enumType = string(fieldDescriptor.Enum().FullName())
		}
		if field.hasDefault {
			field.defaultValue = fieldDescriptor.Default().String()
		}
		result.fields = append(result.fields, field)
	}
	for i := 0; i < msgDesc.Oneofs().Len(); i++ {
		od := msgDesc.Oneofs().Get(i)
		oneof := reflectedOneof{name: string(od.Name()), fields: make([]string, 0, od.Fields().Len())}
		for j := 0; j < od.Fields().Len(); j++ {
			oneof.fields = append(oneof.fields, string(od.Fields().Get(j).Name()))
		}
		result.oneofs = append(result.oneofs, oneof)
	}
	return result, nil
}

func (m *Module) messageDescriptionValue(description *reflectedMessage) goja.Value {
	obj := m.runtime.NewObject()
	_ = obj.Set("name", description.name)
	fields := m.runtime.NewArray()
	for i, field := range description.fields {
		fieldObject := m.runtime.NewObject()
		_ = fieldObject.Set("name", field.name)
		_ = fieldObject.Set("number", field.number)
		_ = fieldObject.Set("type", field.kind)
		_ = fieldObject.Set("repeated", field.repeated)
		_ = fieldObject.Set("map", field.mapField)
		if field.messageType != "" {
			_ = fieldObject.Set("messageType", field.messageType)
		}
		if field.enumType != "" {
			_ = fieldObject.Set("enumType", field.enumType)
		}
		if field.hasDefault {
			_ = fieldObject.Set("defaultValue", field.defaultValue)
		}
		_ = fields.Set(intStr(i), fieldObject)
	}
	_ = obj.Set("fields", fields)
	oneofs := m.runtime.NewArray()
	for i, oneof := range description.oneofs {
		oneofObject := m.runtime.NewObject()
		_ = oneofObject.Set("name", oneof.name)
		oneofFields := m.runtime.NewArray()
		for j, name := range oneof.fields {
			_ = oneofFields.Set(intStr(j), name)
		}
		_ = oneofObject.Set("fields", oneofFields)
		_ = oneofs.Set(intStr(i), oneofObject)
	}
	_ = obj.Set("oneofs", oneofs)
	return obj
}

// fetchFileDescriptorForSymbol retrieves the FileDescriptorSet containing
// the given symbol via the gRPC reflection service.
func (m *Module) fetchFileDescriptorForSymbol(symbol string) (*descriptorpb.FileDescriptorSet, error) {
	return m.fetchFileDescriptorForSymbolContext(m.ctx, symbol)
}

func (m *Module) fetchFileDescriptorForSymbolContext(ctx context.Context, symbol string) (*descriptorpb.FileDescriptorSet, error) {
	return fetchFileDescriptorForSymbolContext(ctx, m.channel, symbol)
}

func fetchFileDescriptorForSymbolContext(
	ctx context.Context,
	channel grpc.ClientConnInterface,
	symbol string,
) (*descriptorpb.FileDescriptorSet, error) {
	client := reflectionpb.NewServerReflectionClient(channel)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.CloseSend()

	err = stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: symbol,
		},
	})
	if err != nil {
		return nil, err
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	if err := reflectionResponseError(resp); err != nil {
		return nil, err
	}

	fdResp := resp.GetFileDescriptorResponse()
	if fdResp == nil {
		return nil, errReflectionFailed("unexpected response type for symbol " + symbol)
	}

	fds := &descriptorpb.FileDescriptorSet{}
	for _, fdBytes := range fdResp.FileDescriptorProto {
		fdp := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(fdBytes, fdp); err != nil {
			return nil, err
		}
		fds.File = append(fds.File, fdp)
	}

	// Fetch transitive dependencies by iterating until we have all files.
	// The first response only contains directly relevant files.
	// We need to resolve dependency imports.
	resolved := make(map[string]bool)
	for _, f := range fds.File {
		resolved[f.GetName()] = true
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var missing []string
		queued := make(map[string]bool)
		for _, f := range fds.File {
			for _, dep := range f.Dependency {
				if !resolved[dep] && !queued[dep] {
					queued[dep] = true
					missing = append(missing, dep)
				}
			}
		}
		if len(missing) == 0 {
			break
		}
		for _, depName := range missing {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			err = stream.Send(&reflectionpb.ServerReflectionRequest{
				MessageRequest: &reflectionpb.ServerReflectionRequest_FileByFilename{
					FileByFilename: depName,
				},
			})
			if err != nil {
				return nil, err
			}
			resp, err = stream.Recv()
			if err != nil {
				return nil, err
			}
			if err := reflectionResponseError(resp); err != nil {
				return nil, err
			}
			fdResp = resp.GetFileDescriptorResponse()
			if fdResp == nil {
				return nil, errReflectionFailed("unexpected response type for dependency " + depName)
			}
			added := false
			for _, fdBytes := range fdResp.FileDescriptorProto {
				fdp := &descriptorpb.FileDescriptorProto{}
				if err := proto.Unmarshal(fdBytes, fdp); err != nil {
					return nil, err
				}
				if resolved[fdp.GetName()] {
					continue
				}
				resolved[fdp.GetName()] = true
				fds.File = append(fds.File, fdp)
				added = true
			}
			if !resolved[depName] || !added {
				return nil, errReflectionFailed("dependency not returned: " + depName)
			}
		}
	}

	return fds, nil
}

func reflectionResponseError(
	response *reflectionpb.ServerReflectionResponse,
) error {
	if response == nil {
		return errReflectionFailed("nil reflection response")
	}
	responseError := response.GetErrorResponse()
	if responseError == nil {
		return nil
	}
	return status.Error(
		codes.Code(responseError.GetErrorCode()),
		responseError.GetErrorMessage(),
	)
}

// intStr converts an int to a string for array indexing.
func intStr(i int) string {
	return strconv.Itoa(i)
}

// errReflectionFailed creates a reflection-specific error.
func errReflectionFailed(msg string) error {
	return &reflectionError{msg: msg}
}

type reflectionError struct {
	msg string
}

func (e *reflectionError) Error() string {
	return "grpc reflection: " + e.msg
}
