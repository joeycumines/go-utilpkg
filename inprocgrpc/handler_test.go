package inprocgrpc

import (
	"testing"

	"google.golang.org/grpc"
)

type testHandlerIface interface {
	DoSomething()
}

type validHandler struct{}

func (validHandler) DoSomething() {}

type invalidHandler struct{}

func TestHandlerMap_QueryService_Empty(t *testing.T) {
	var m handlerMap
	sd, h := m.queryService("nonexistent")
	if sd != nil || h != nil {
		t.Errorf("expected nil, got %v, %v", sd, h)
	}
}

func TestHandlerMap_GetServiceInfo_Empty(t *testing.T) {
	var m handlerMap
	info := m.getServiceInfo()
	if info != nil {
		t.Errorf("expected nil, got %v", info)
	}
}

func TestHandlerMap_RegisterAndQuery(t *testing.T) {
	var m handlerMap
	desc := grpc.ServiceDesc{
		ServiceName: "test.Svc",
		Methods: []grpc.MethodDesc{
			{MethodName: "Foo"},
		},
		Streams: []grpc.StreamDesc{
			{StreamName: "Bar", ServerStreams: true},
		},
	}
	handler := struct{}{}
	m.registerService(&desc, handler)

	sd, h := m.queryService("test.Svc")
	if sd == nil {
		t.Fatal("service not found")
	}
	if sd.ServiceName != "test.Svc" {
		t.Errorf("got %q", sd.ServiceName)
	}
	if h == nil {
		t.Error("handler is nil")
	}

	info := m.getServiceInfo()
	si, ok := info["test.Svc"]
	if !ok {
		t.Fatal("service info not found")
	}
	if len(si.Methods) != 2 { // 1 unary + 1 stream
		t.Errorf("got %d methods", len(si.Methods))
	}
}

func TestHandlerMap_RegisterDuplicate(t *testing.T) {
	var m handlerMap
	desc := grpc.ServiceDesc{ServiceName: "test.Dup"}
	m.registerService(&desc, struct{}{})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	m.registerService(&desc, struct{}{})
}

func TestHandlerMap_RegisterService_HandlerTypeValidation(t *testing.T) {
	var m handlerMap
	desc := grpc.ServiceDesc{
		ServiceName: "test.Valid",
		HandlerType: (*testHandlerIface)(nil),
	}
	// Valid handler should not panic
	m.registerService(&desc, validHandler{})
	sd, h := m.queryService("test.Valid")
	if sd == nil || h == nil {
		t.Error("valid handler not registered")
	}
}

func TestHandlerMap_RegisterService_InvalidHandlerType(t *testing.T) {
	var m handlerMap
	desc := grpc.ServiceDesc{
		ServiceName: "test.Invalid",
		HandlerType: (*testHandlerIface)(nil),
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid handler type")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is not string: %T", r)
		}
		if msg == "" {
			t.Error("panic message is empty")
		}
	}()
	m.registerService(&desc, invalidHandler{})
}

func TestHandlerMap_RegisterService_NilHandlerType(t *testing.T) {
	var m handlerMap
	desc := grpc.ServiceDesc{
		ServiceName: "test.NoType",
		// HandlerType is nil, validation should be skipped
	}
	m.registerService(&desc, struct{}{})
	sd, _ := m.queryService("test.NoType")
	if sd == nil {
		t.Error("service not registered")
	}
}
