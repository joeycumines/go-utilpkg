# gRPC Reflection Design — Research Notes

## Overview

gRPC Server Reflection enables clients to discover and interact with services
without pre-loaded proto descriptors. This is essential for tooling like
`grpcurl` and for dynamic client workflows.

## API Surface

### Server Side

```go
import "google.golang.org/grpc/reflection"

// GRPCServer interface (our Channel satisfies this)
type GRPCServer interface {
    grpc.ServiceRegistrar       // RegisterService(*grpc.ServiceDesc, any)
    ServiceInfoProvider          // GetServiceInfo() map[string]grpc.ServiceInfo
}

// Register the reflection service
reflection.Register(channel)  // channel is *inprocgrpc.Channel
```

Our `inprocgrpc.Channel` already implements `GRPCServer`:
- `RegisterService` ✅ (core functionality)
- `GetServiceInfo` ✅ (returns registered service metadata)

### Service Definition

The reflection service (`grpc.reflection.v1`) has one bidi-streaming method:

```proto
service ServerReflection {
    rpc ServerReflectionInfo(stream ServerReflectionRequest)
        returns (stream ServerReflectionResponse);
}
```

### Request Types

1. **ListServices**: Returns all registered service names
2. **FileByFilename**: Returns FileDescriptorProto for a given .proto file
3. **FileContainingSymbol**: Returns FileDescriptorProto containing a given symbol
4. **FileContainingExtension**: Returns FileDescriptorProto for extension
5. **AllExtensionNumbersOfType**: Returns all extension numbers for a type

### Response

All responses include:
- `file_descriptor_response`: Serialized FileDescriptorProto bytes
- `list_services_response`: List of service names
- `error_response`: Error details

## JS API Design

### Option A: Dedicated Reflection Client (Chosen)

```javascript
const grpc = require('grpc');

// Create a reflection-aware client
const reflector = grpc.createReflectionClient(channel);

// Discovery
const services = await reflector.listServices();
// => ['testgrpc.TestService', 'grpc.reflection.v1.ServerReflection']

const svcDesc = await reflector.describeService('testgrpc.TestService');
// => { name: 'testgrpc.TestService', methods: [...] }

const typeDesc = await reflector.describeType('testgrpc.EchoRequest');
// => { name: 'testgrpc.EchoRequest', fields: [...] }
```

### Option B: Extension on existing client (Rejected)

```javascript
const client = grpc.createClient('testgrpc.TestService');
const methods = await client.reflect.getMethods(); // mixing concerns
```

Rejected because: reflection is channel-level, not service-level.

## Implementation Plan

1. **T203**: Register `reflection.Register(channel)` on inprocgrpc Channel
   - The reflection service registers as a standard gRPC service
   - No special code needed — just call `reflection.Register(ch)` after service registration
   - The reflection server auto-discovers registered services via `GetServiceInfo()`

2. **T199-T202**: Implement JS reflection client
   - Uses the same bidi-streaming RPC mechanism already implemented in goja-grpc
   - Opens a `ServerReflectionInfo` stream
   - Sends request messages, receives response messages
   - Parses FileDescriptorProto bytes using `protodesc`
   - Returns structured JS objects

3. **Key dependency**: The reflection service sends FileDescriptorProto as bytes.
   The JS client needs to:
   a. Receive the bytes via the bidi stream
   b. Parse them using `proto.Unmarshal` (Go side, via goja-protobuf)
   c. Convert to structured JS objects (service descriptors, method info)

## Integration with inprocgrpc

The reflection service is a standard gRPC service. Once registered via
`reflection.Register(ch)`, it's served like any other service. No changes
to inprocgrpc are needed.

The key insight: `reflection.Register(ch)` works because our Channel
implements the `reflection.GRPCServer` interface. The reflection server
calls `ch.GetServiceInfo()` to enumerate services, and the standard
`grpc.ServiceDesc` metadata provides method-level details.

## Limitations

1. `GetServiceInfo()` only returns service metadata, not full proto descriptors.
   To get full FileDescriptorProto for field-level introspection, the server
   needs ProtoReflect descriptors. Our Channel doesn't store these — it only
   stores `grpc.ServiceDesc`. The `reflection` package handles this by using
   `protoregistry.GlobalFiles` as a fallback. Services registered with compiled
   proto stubs are automatically found. Services registered with dynamicpb
   descriptors may need explicit registration.

2. For goja-grpc's dynamicpb-based services, descriptors ARE already loaded
   via `LoadDescriptorSetBytes()`. We need to ensure these are registered
   in `protoregistry.GlobalFiles` or provided to the reflection server via
   `ServerOptions.Services`.
