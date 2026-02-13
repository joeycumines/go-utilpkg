package inprocgrpc

import (
	"fmt"
	"reflect"
	"sync"

	"google.golang.org/grpc"
)

// handlerMap accumulates service handlers into a map.
// It implements grpc.ServiceRegistrar.
type handlerMap struct {
	services map[string]serviceEntry
	mu       sync.RWMutex
}

type serviceEntry struct {
	desc    *grpc.ServiceDesc
	handler any
}

// registerService registers a service handler. Panics if the handler does not
// implement the service's HandlerType, or if a handler is already registered
// for the service.
func (m *handlerMap) registerService(desc *grpc.ServiceDesc, impl any) {
	if desc.HandlerType != nil {
		ht := reflect.TypeOf(desc.HandlerType).Elem()
		st := reflect.TypeOf(impl)
		if !st.Implements(ht) {
			panic(fmt.Sprintf("inprocgrpc: handler of type %v does not satisfy %v", st, ht))
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.services == nil {
		m.services = make(map[string]serviceEntry)
	}
	if _, ok := m.services[desc.ServiceName]; ok {
		panic(fmt.Sprintf("inprocgrpc: service %q already registered", desc.ServiceName))
	}
	m.services[desc.ServiceName] = serviceEntry{desc: desc, handler: impl}
}

// queryService returns the service descriptor and handler for the given service
// name. Returns nil, nil if not found.
func (m *handlerMap) queryService(name string) (*grpc.ServiceDesc, any) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.services == nil {
		return nil, nil
	}
	e, ok := m.services[name]
	if !ok {
		return nil, nil
	}
	return e.desc, e.handler
}

// getServiceInfo returns service information compatible with grpc.ServiceInfo.
func (m *handlerMap) getServiceInfo() map[string]grpc.ServiceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.services == nil {
		return nil
	}
	info := make(map[string]grpc.ServiceInfo, len(m.services))
	for name, entry := range m.services {
		methods := make([]grpc.MethodInfo, 0, len(entry.desc.Methods)+len(entry.desc.Streams))
		for _, md := range entry.desc.Methods {
			methods = append(methods, grpc.MethodInfo{
				Name:           md.MethodName,
				IsClientStream: false,
				IsServerStream: false,
			})
		}
		for _, sd := range entry.desc.Streams {
			methods = append(methods, grpc.MethodInfo{
				Name:           sd.StreamName,
				IsClientStream: sd.ClientStreams,
				IsServerStream: sd.ServerStreams,
			})
		}
		info[name] = grpc.ServiceInfo{
			Methods:  methods,
			Metadata: entry.desc.Metadata,
		}
	}
	return info
}
