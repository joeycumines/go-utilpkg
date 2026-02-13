package gojagrpc

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/dop251/goja"
	"google.golang.org/grpc/reflection"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
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
	obj := m.runtime.NewObject()
	_ = obj.Set("listServices", m.runtime.ToValue(m.jsReflListServices))
	_ = obj.Set("describeService", m.runtime.ToValue(m.jsReflDescribeService))
	_ = obj.Set("describeType", m.runtime.ToValue(m.jsReflDescribeType))
	return obj
}

// EnableReflection registers the gRPC reflection service on the
// underlying channel, enabling JS clients to discover services
// and types at runtime. It uses the protobuf module's file resolver
// so dynamically loaded descriptors are visible to reflection.
//
// This should be called AFTER all services have been registered.
func (m *Module) EnableReflection() {
	reflServer := reflection.NewServerV1(reflection.ServerOptions{
		Services:           m.channel,
		DescriptorResolver: m.protobuf.FileResolver(),
	})
	reflectionpb.RegisterServerReflectionServer(m.channel, reflServer)
}

// jsReflListServices returns a Promise<string[]> of all registered service names.
func (m *Module) jsReflListServices(call goja.FunctionCall) goja.Value {
	promise, resolve, reject := m.adapter.JS().NewChainedPromise()

	go func() {
		services, err := m.doListServices()
		if submitErr := m.adapter.Loop().Submit(func() {
			if err != nil {
				reject(m.runtime.NewGoError(err))
				return
			}
			arr := m.runtime.NewArray()
			for i, s := range services {
				_ = arr.Set(intStr(i), m.runtime.ToValue(s))
			}
			resolve(arr)
		}); submitErr != nil {
			reject(fmt.Errorf("event loop not running"))
		}
	}()

	return m.adapter.GojaWrapPromise(promise)
}

// jsReflDescribeService returns a Promise<{name, methods: [...]}>
// for a given fully-qualified service name.
func (m *Module) jsReflDescribeService(call goja.FunctionCall) goja.Value {
	name := call.Argument(0).String()
	promise, resolve, reject := m.adapter.JS().NewChainedPromise()

	go func() {
		desc, err := m.doDescribeService(name)
		if submitErr := m.adapter.Loop().Submit(func() {
			if err != nil {
				reject(m.runtime.NewGoError(err))
				return
			}
			resolve(desc)
		}); submitErr != nil {
			reject(fmt.Errorf("event loop not running"))
		}
	}()

	return m.adapter.GojaWrapPromise(promise)
}

// jsReflDescribeType returns a Promise<{name, fields: [...]}>
// for a given fully-qualified message type name.
func (m *Module) jsReflDescribeType(call goja.FunctionCall) goja.Value {
	name := call.Argument(0).String()
	promise, resolve, reject := m.adapter.JS().NewChainedPromise()

	go func() {
		desc, err := m.doDescribeType(name)
		if submitErr := m.adapter.Loop().Submit(func() {
			if err != nil {
				reject(m.runtime.NewGoError(err))
				return
			}
			resolve(desc)
		}); submitErr != nil {
			reject(fmt.Errorf("event loop not running"))
		}
	}()

	return m.adapter.GojaWrapPromise(promise)
}

// doListServices uses the Go reflection client to list all services.
func (m *Module) doListServices() ([]string, error) {
	client := reflectionpb.NewServerReflectionClient(m.channel)
	stream, err := client.ServerReflectionInfo(context.Background())
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
func (m *Module) doDescribeService(name string) (goja.Value, error) {
	fd, err := m.fetchFileDescriptorForSymbol(name)
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

	// Build JS object.
	obj := m.runtime.NewObject()
	_ = obj.Set("name", m.runtime.ToValue(string(svcDesc.FullName())))

	methods := m.runtime.NewArray()
	for i := 0; i < svcDesc.Methods().Len(); i++ {
		md := svcDesc.Methods().Get(i)
		mObj := m.runtime.NewObject()
		_ = mObj.Set("name", m.runtime.ToValue(string(md.Name())))
		_ = mObj.Set("fullName", m.runtime.ToValue(string(md.FullName())))
		_ = mObj.Set("inputType", m.runtime.ToValue(string(md.Input().FullName())))
		_ = mObj.Set("outputType", m.runtime.ToValue(string(md.Output().FullName())))
		_ = mObj.Set("clientStreaming", m.runtime.ToValue(md.IsStreamingClient()))
		_ = mObj.Set("serverStreaming", m.runtime.ToValue(md.IsStreamingServer()))
		_ = methods.Set(intStr(i), mObj)
	}
	_ = obj.Set("methods", methods)

	return obj, nil
}

// doDescribeType retrieves message type metadata via reflection.
func (m *Module) doDescribeType(name string) (goja.Value, error) {
	fd, err := m.fetchFileDescriptorForSymbol(name)
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

	// Build JS object with fields array.
	obj := m.runtime.NewObject()
	_ = obj.Set("name", m.runtime.ToValue(string(msgDesc.FullName())))

	fields := m.runtime.NewArray()
	for i := 0; i < msgDesc.Fields().Len(); i++ {
		fd := msgDesc.Fields().Get(i)
		fObj := m.runtime.NewObject()
		_ = fObj.Set("name", m.runtime.ToValue(string(fd.Name())))
		_ = fObj.Set("number", m.runtime.ToValue(int(fd.Number())))
		_ = fObj.Set("type", m.runtime.ToValue(fd.Kind().String()))
		_ = fObj.Set("repeated", m.runtime.ToValue(fd.IsList()))
		_ = fObj.Set("map", m.runtime.ToValue(fd.IsMap()))

		if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
			_ = fObj.Set("messageType", m.runtime.ToValue(string(fd.Message().FullName())))
		}
		if fd.Kind() == protoreflect.EnumKind {
			_ = fObj.Set("enumType", m.runtime.ToValue(string(fd.Enum().FullName())))
		}
		if fd.HasDefault() {
			_ = fObj.Set("defaultValue", m.runtime.ToValue(fd.Default().String()))
		}

		_ = fields.Set(intStr(i), fObj)
	}
	_ = obj.Set("fields", fields)

	// Oneofs.
	oneofs := m.runtime.NewArray()
	for i := 0; i < msgDesc.Oneofs().Len(); i++ {
		od := msgDesc.Oneofs().Get(i)
		oObj := m.runtime.NewObject()
		_ = oObj.Set("name", m.runtime.ToValue(string(od.Name())))
		oFields := m.runtime.NewArray()
		for j := 0; j < od.Fields().Len(); j++ {
			_ = oFields.Set(intStr(j), m.runtime.ToValue(string(od.Fields().Get(j).Name())))
		}
		_ = oObj.Set("fields", oFields)
		_ = oneofs.Set(intStr(i), oObj)
	}
	_ = obj.Set("oneofs", oneofs)

	return obj, nil
}

// fetchFileDescriptorForSymbol retrieves the FileDescriptorSet containing
// the given symbol via the gRPC reflection service.
func (m *Module) fetchFileDescriptorForSymbol(symbol string) (*descriptorpb.FileDescriptorSet, error) {
	client := reflectionpb.NewServerReflectionClient(m.channel)
	stream, err := client.ServerReflectionInfo(context.Background())
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

	if errResp := resp.GetErrorResponse(); errResp != nil {
		return nil, errReflectionFailed(errResp.ErrorMessage)
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
		var missing []string
		for _, f := range fds.File {
			for _, dep := range f.Dependency {
				if !resolved[dep] {
					missing = append(missing, dep)
				}
			}
		}
		if len(missing) == 0 {
			break
		}
		for _, depName := range missing {
			resolved[depName] = true
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
				if err == io.EOF {
					break
				}
				return nil, err
			}
			fdResp = resp.GetFileDescriptorResponse()
			if fdResp == nil {
				continue
			}
			for _, fdBytes := range fdResp.FileDescriptorProto {
				fdp := &descriptorpb.FileDescriptorProto{}
				if err := proto.Unmarshal(fdBytes, fdp); err != nil {
					return nil, err
				}
				if !resolved[fdp.GetName()] {
					resolved[fdp.GetName()] = true
				}
				fds.File = append(fds.File, fdp)
			}
		}
	}

	return fds, nil
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
