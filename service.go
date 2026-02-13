package gojagrpc

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// resolveService looks up a [protoreflect.ServiceDescriptor] by
// its fully-qualified name using the protobuf module's descriptor
// registries.
func (m *Module) resolveService(serviceName string) (protoreflect.ServiceDescriptor, error) {
	desc, err := m.protobuf.FindDescriptor(protoreflect.FullName(serviceName))
	if err != nil {
		return nil, fmt.Errorf("service %q not found: %w", serviceName, err)
	}
	sd, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is a %T, not a service descriptor", serviceName, desc)
	}
	return sd, nil
}

// lowerFirst returns s with its first character lowercased. This
// converts PascalCase proto method names (e.g. "SayHello") to
// lowerCamelCase JS method names (e.g. "sayHello").
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
