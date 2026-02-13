# Error Handling Guide

## Creating Errors

### JavaScript

```javascript
// Create a gRPC error with status code
throw grpc.status.createError(grpc.status.NOT_FOUND, 'item not found');

// Status codes available
grpc.status.OK
grpc.status.CANCELLED
grpc.status.UNKNOWN
grpc.status.INVALID_ARGUMENT
grpc.status.DEADLINE_EXCEEDED
grpc.status.NOT_FOUND
grpc.status.ALREADY_EXISTS
grpc.status.PERMISSION_DENIED
grpc.status.RESOURCE_EXHAUSTED
grpc.status.FAILED_PRECONDITION
grpc.status.ABORTED
grpc.status.OUT_OF_RANGE
grpc.status.UNIMPLEMENTED
grpc.status.INTERNAL
grpc.status.UNAVAILABLE
grpc.status.DATA_LOSS
grpc.status.UNAUTHENTICATED
```

## Error Propagation

### Server → Client

When a server handler throws or returns an error, it's automatically converted to a
gRPC status error and propagated to the client:

```javascript
// Server
server.addService('myservice.MyService', {
    myMethod: function(request, call) {
        if (!request.get('name')) {
            throw grpc.status.createError(grpc.status.INVALID_ARGUMENT, 'name required');
        }
        // ...
    }
});

// Client receives the error
client.myMethod(req).catch(function(err) {
    console.log(err.code);    // 3 (INVALID_ARGUMENT)
    console.log(err.message); // 'name required'
});
```

### Go → JS

Go errors returned from `channel.Invoke()` or stream operations are standard
`google.golang.org/grpc/status` errors:

```go
err := channel.Invoke(ctx, method, req, resp)
if err != nil {
    st := status.Convert(err)
    fmt.Println(st.Code())    // codes.NotFound
    fmt.Println(st.Message()) // "item not found"
}
```

## AbortSignal Cancellation

RPCs can be cancelled using `AbortController`:

```javascript
var controller = new AbortController();
var signal = controller.signal;

client.myMethod(req, { signal: signal }).catch(function(err) {
    console.log(err.code); // CANCELLED
});

// Cancel the RPC
controller.abort();
```

### Pre-aborted signals

If the signal is already aborted, the RPC rejects immediately:

```javascript
var controller = new AbortController();
controller.abort(); // Already aborted

client.myMethod(req, { signal: controller.signal }).catch(function(err) {
    // Immediately rejected with CANCELLED
});
```

## Panic Handling

If a JS handler throws a non-GrpcError exception, it's wrapped as an INTERNAL error:

```javascript
server.addService('myservice.MyService', {
    myMethod: function(request, call) {
        throw new Error('unexpected'); // Becomes INTERNAL: unexpected
    }
});
```
