# goja-protobuf Benchmark Results

**Platform:** macOS (Apple M2 Pro, arm64)
**Go Version:** 1.25.7

## JS vs Native Go — Proto Operations

| Operation | JS (ns/op) | Go (ns/op) | Overhead | JS allocs | Go allocs |
|-----------|-----------|-----------|----------|-----------|-----------|
| Message create | 4,497 | 67 | **67x** | 106 | 3 |
| Field get+set | 2,746 | 46 | **60x** | 66 | 1 |
| Binary encode+decode | 3,491 | 762 | **4.6x** | 76 | 17 |
| JSON encode+decode | 3,414 | — | — | 76 | — |

### Analysis

**Message creation** (67x overhead): The JS path creates goja objects, property descriptors,
and wraps the underlying `dynamicpb.Message`. This is the highest overhead because goja
must set up the full proxy object with getters/setters for each field.

**Field access** (60x overhead): A `msg.set('name', 'test'); msg.get('name')` in JS involves
goja property lookup, type coercion, and dynamic dispatch through the protobuf bridge.
The native Go path is a direct `msg.Set(field, value)` call with no reflection.

**Binary encode/decode** (4.6x overhead): This is the most favorable comparison. The actual
proto serialization is done by `proto.Marshal`/`proto.Unmarshal` in both paths. The JS
overhead is limited to ArrayBuffer creation and message wrapper construction.

## Collection Operations

| Operation | JS (ns/op) | B/op | allocs/op |
|-----------|-----------|------|-----------|
| Repeated field (5 items, set+iterate) | 8,161 | 9,750 | 171 |
| Map field (3 entries, set+lookup) | 7,241 | 9,150 | 167 |

Repeated and map fields involve JS array/object conversion, which multiplies the
per-element overhead. Each element requires type conversion through the bridge.

## Allocation Breakdown

| Component | Message create | Field get/set | Encode/decode |
|-----------|---------------|---------------|---------------|
| goja internals | ~70 | ~45 | ~35 |
| Proto bridge | ~30 | ~15 | ~25 |
| dynamicpb | ~6 | ~6 | ~16 |
| **Total** | **106** | **66** | **76** |

## Practical Implications

Despite the overhead multipliers, the absolute times are reasonable for scripting workloads:

- Creating 1000 messages: ~4.5ms (JS) vs ~0.07ms (Go)
- Setting 1000 fields: ~2.7ms (JS) vs ~0.05ms (Go)
- Encoding 1000 messages: ~3.5ms (JS) vs ~0.8ms (Go)

For typical gRPC workloads (10-100 messages per request), the proto overhead adds
<1ms to total processing time. The RPC mechanism overhead (inprocgrpc channel, event
loop scheduling) dominates at ~5µs per call.
