# ADR: Standardizing on `weak.Pointer[T]` Key Identity for Weak Maps and GC-Observing Collections

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-08-22 |
| **Scope** | All present and future weak maps, caches, registries, and GC-observing collections across the project |
| **Standard** | Go 1.24+ (`weak` package & `runtime.AddCleanup`) |

---

## Summary

Any collection or registry that observes object lifecycle and automatically evicts entries upon garbage collection must identify objects with permanent, collision-free tokens. Identifying reclaimed objects by raw memory addresses introduces race conditions—notably **stale reads** and **premature deletions**—due to allocator address reuse (the ABA problem).

This ADR establishes **`weak.Pointer[T]` as the mandatory, standard key identity mechanism** for all weak maps, associative caches, and GC-observing collections. Alternative approaches—such as raw pointers, `uintptr` addresses, or hand-rolled monotonic struct IDs—are rejected in favor of the idiomatic runtime-supported weak pointer pattern.

---

## Context & Problem Statement

### Associating State with Ephemeral Object Lifecycles

In systems programming and runtime-adjacent tooling, it is frequently necessary to associate auxiliary state with an object without extending that object's lifetime. Common use cases include:

1. **Weak Maps & Sets**: Key-value collections where keys do not prevent GC reclamation of the underlying target.
2. **Canonicalization & Deduplication Caches**: Tables retaining cached representations only as long as consumers hold active references to the source object.
3. **Cross-Boundary Registries**: Metadata tables attached to runtime objects across subsystem boundaries where modifying the object's struct definition is impossible or undesirable.

When an object dies, its corresponding entries in these backing structures must eventually be reclaimed to prevent unbounded memory leaks. In Go 1.24+, this reclamation is driven by `runtime.AddCleanup`.

### The Hazard: Allocator Address Reuse & The ABA Problem

Go employs a non-moving garbage collector: once an object is allocated, its memory address remains invariant for the duration of its lifetime [1]. However, once an object becomes unreachable and is swept, its memory address is returned to the allocator pool and **will be reused for subsequent, unrelated allocations**.

A map or cleanup handler that identifies an entry by its raw memory address (`*T` equality after free, or `uintptr(unsafe.Pointer(obj))`) suffers from the classic **ABA problem** [2]:

```
Timeline:
1. Object A allocated at address 0x1000.
2. Map associates 0x1000 -> Value A. Cleanup C_A registered for Object A.
3. Object A becomes unreachable and is collected by GC. Address 0x1000 is freed.
4. (Race window begins: Cleanup C_A has not yet executed).
5. New, unrelated Object B is allocated at the recycled address 0x1000.
6. Map associates 0x1000 -> Value B. Cleanup C_B registered for Object B.
7. Cleanup C_A finally runs for Object A and executes: delete(map, 0x1000).
```

This sequence triggers severe correctness failures:

- **Premature Deletion (Ghost Eviction)**: Cleanup `C_A` deletes the entry at `0x1000`, erroneously destroying the live, valid entry belonging to Object B.
- **Stale Reads & State Pollution**: If Object B queries the map before step 6, it reads `Value A` belonging to the deallocated Object A.
- **Cross-Talk / Memory Corruption**: Unrelated components inadvertently mutate or observe state belonging to different object lifecycles.

### Cleanup Execution Concurrency

Cleanups registered with `runtime.AddCleanup` execute asynchronously on runtime goroutines [6]. Starting in Go 1.25, cleanups also run concurrently and in parallel with one another [7]. Because cleanup execution is decoupled from the exact instant of object death, **the delay between an address being recycled and its prior cleanup handler executing is non-zero and non-deterministic**. 

Consequently, any design relying on memory addresses or non-unique keys is fundamentally racy.

---

## Decision

We standardize on **`weak.Pointer[T]`** (via `weak.Make` from the Go standard library) paired with **`runtime.AddCleanup`** as the required identity mechanism for all weak maps, associative caches, and GC-observing collections.

### Core Architectural Rules

1. **Weak Pointer as Key Identity**: Backing maps for weak collections must be keyed directly by `weak.Pointer[K]` (e.g., `map[weak.Pointer[K]]V` or concurrent variants), where `K` is the referent object type.
2. **Cleanup Token Binding**: When registering cleanups via `runtime.AddCleanup(target, cleanupFn, token)`, the cleanup token passed must be the `weak.Pointer[K]` (or a value containing it).
3. **Exact Identity Eviction**: Cleanups must evict entries using the captured `weak.Pointer[K]`. Because `weak.Pointer[T]` maintains referent equality even after reclamation [3], evicting by weak pointer is mathematically immune to address recycling.
4. **Prohibition of Address-Derived Keys in GC-Observing Maps**: Raw pointers (`*T`), raw addresses (`uintptr`), or pointer hashes must never be used as map keys if entries are subject to asynchronous GC-driven deletion.
5. **Retirement of Bespoke ID Counters**: Hand-rolled monotonic counters embedded in structs (e.g., `uint64` object IDs) are deprecated for GC-observing identity in favor of the standard `weak.Pointer[T]` pattern.

---

## Technical Design & Canonical Pattern

### How `weak.Pointer[T]` Solves Identity

The Go runtime defines weak pointer equality over referent identity across time:

> *"Two `Pointer` values compare equal if and only if the pointers from which they were created compare equal. This property is maintained even after the object referenced by the pointer used to create a weak reference is reclaimed."* [3]

If Object A at address `0x1000` dies, and Object B is subsequently allocated at address `0x1000`:
$$\text{weak.Make}(A) \neq \text{weak.Make}(B)$$

Even though their underlying addresses match, their `weak.Pointer` values remain distinct. This eliminates ABA address-reuse hazards by construction.

### Canonical Implementation: `WeakMap[K, V]`

The standard pattern for an associative weak map is structured as follows:

```go
package collections

import (
	"runtime"
	"sync"
	"weak"
)

// WeakMap associates pointer keys of type *K with values of type V
// without preventing *K from being garbage collected.
type WeakMap[K any, V any] struct {
	mu sync.RWMutex
	m  map[weak.Pointer[K]]V
}

func NewWeakMap[K any, V any]() *WeakMap[K, V] {
	return &WeakMap[K, V]{
		m: make(map[weak.Pointer[K]]V),
	}
}

// Set stores a value associated with key. When key is reclaimed by GC,
// the entry is automatically evicted.
func (w *WeakMap[K, V]) Set(key *K, val V) {
	if key == nil {
		panic("WeakMap.Set: key cannot be nil")
	}

	wp := weak.Make(key)

	w.mu.Lock()
	w.m[wp] = val
	w.mu.Unlock()

	// Register cleanup to evict the entry when key dies.
	// Passing wp as the cleanup argument ensures safe identity closure.
	runtime.AddCleanup(key, func(targetWP weak.Pointer[K]) {
		w.mu.Lock()
		delete(w.m, targetWP)
		w.mu.Unlock()
	}, wp)
}

// Get retrieves the value associated with key.
func (w *WeakMap[K, V]) Get(key *K) (V, bool) {
	if key == nil {
		var zero V
		return zero, false
	}

	wp := weak.Make(key)

	w.mu.RLock()
	val, ok := w.m[wp]
	w.mu.RUnlock()

	return val, ok
}

// Delete explicitly removes the key association.
func (w *WeakMap[K, V]) Delete(key *K) {
	if key == nil {
		return
	}

	wp := weak.Make(key)

	w.mu.Lock()
	delete(w.m, wp)
	w.mu.Unlock()
}
```

### Canonical Implementation: Concurrent Cache with Compare-and-Delete

For lock-free or concurrent-map caches, the compare-and-delete idiom ensures exact lifecycle synchronization [5]:

```go
type DeduplicationCache[K comparable, V any] struct {
	cache sync.Map // map[K]weak.Pointer[V]
}

func (c *DeduplicationCache[K, V]) GetOrCompute(key K, compute func(K) *V) *V {
	for {
		if val, ok := c.cache.Load(key); ok {
			wp := val.(weak.Pointer[V])
			if ptr := wp.Value(); ptr != nil {
				return ptr
			}
		}

		newVal := compute(key)
		wp := weak.Make(newVal)

		actual, loaded := c.cache.LoadOrStore(key, wp)
		if !loaded {
			runtime.AddCleanup(newVal, func(storedWP weak.Pointer[V]) {
				// CompareAndDelete ensures that if a new entry was already stored
				// under the same key, the new entry is NOT evicted.
				c.cache.CompareAndDelete(key, storedWP)
			}, wp)
			return newVal
		}

		if ptr := actual.(weak.Pointer[V]).Value(); ptr != nil {
			return ptr
		}
		// Referent died between LoadOrStore and Value(); retry.
	}
}
```

---

## Rationale & Comparative Analysis

| Mechanism | ABA Safety | Struct Invasive? | External Type Support | Synchronization Complexity | Memory Overhead |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`weak.Pointer[T]`** | **Full (guaranteed by runtime)** [3] | **No** (pure side-table) | **Full** (any pointer type) | **Low** (standard map/lock) | ~8 bytes per weak ref indirection [10] |
| **Monotonic Object ID** | Full (counter uniqueness) | **Yes** (adds ID field to struct) | **None** (cannot modify external types) | Medium (atomic ID generation) | 8–16 bytes on *every* instance + padding |
| **Raw Address / `uintptr`** | **Unsafe** (ABA vulnerable) [2] | No | Full | Impossible (racy by design) | 0 bytes |
| **Finalizers (`SetFinalizer`)** | Unsafe / Flawed | No | Partial | High (resurrection, delayed GC) | Runtime finalizer queue |

### Why `weak.Pointer[T]` is the Superior Solution

1. **Runtime-Backed Identity Guarantees**:
   The Go runtime tracks weak pointer identities internally. A reclaimed object's weak reference does not alias future allocations at the same address. This completely eliminates stale reads and premature deletions.

2. **Non-Invasive Architecture**:
   Alternative ABA-safe mechanisms require embedding monotonic 64-bit sequence numbers (e.g., `id uint64`) into every object. This pollutes struct definitions, inflates object footprints across the entire application (including objects never placed in weak maps), and fails when wrapping third-party or standard-library types. `weak.Pointer[T]` operates on any standard Go pointer without altering the target type.

3. **Concurrency Resilience**:
   Because cleanups in Go run on dedicated background goroutines (and in parallel as of Go 1.25 [7]), cleanup logic must safely coordinate with application mutations. Keying by `weak.Pointer[K]` guarantees that regardless of execution ordering or scheduling delays, cleanup actions target only their exact originating instance.

---

## Critical Invariants & Guidelines

When implementing weak collections or GC-observing structures, developers must adhere to the following invariants:

### 1. Avoid Retaining Strong References in Values (Retention Cycle Hazard)

If a value stored in a `WeakMap[K, V]` contains a strong pointer back to key `K` (directly or transitively), `K` will **never become unreachable**, and the cleanup will never run [9].

$$\text{Key } K \longleftarrow \text{strong ref} \longleftarrow \text{Value } V \longleftarrow \text{WeakMap}$$

- **Rule**: Map values must never directly or indirectly retain the map key. If bidirectional association is required, the value must hold a `weak.Pointer[K]` rather than `*K`.

### 2. Cleanups are Eventual, Not Real-Time Finalizers

`runtime.AddCleanup` is designed for memory management and cache eviction [5, 6]. Cleanups are not guaranteed to run promptly upon object death, nor before application exit [6].
- **Rule**: Weak maps must not be used for deterministic release of operating system resources (e.g., file descriptors, database connections). Use explicit lifecycle methods (`Close()`, `io.Closer`) for non-memory resources.

### 3. Cleanup Concurrency Safety

Cleanup handlers run concurrently with main application goroutines [6, 7].
- **Rule**: Any internal map access inside a cleanup handler must be properly synchronized (via mutexes, concurrent maps, or actor channels) against concurrent reads, writes, and other cleanups.

---

## Consequences

### Positive

- **Elimination of ABA Races**: Stale lookups, cross-talk, and ghost evictions caused by address recycling are eliminated across all collections.
- **Unified Architectural Standard**: Replaces disparate, hand-rolled identification schemes (atomic IDs, side-tables, address hashes) with a single idiomatic pattern.
- **Zero Struct Overhead**: Objects managed by weak maps remain clean structs without dummy identity fields or unnecessary memory padding.
- **Broad Compatibility**: Weak maps can be constructed over any pointer type, including types defined in external libraries.

### Negative & Trade-offs

- **Indirection Allocation**: `weak.Make` incurs a small runtime allocation (~8 bytes) for the internal weak reference descriptor [10]. This trade-off is negligible compared to the correctness guarantees and the struct footprint savings across unmanaged instances.
- **Toolchain Requirement**: Requires Go 1.24 or higher [12].

---

## References

1. Go Team, *A Guide to the Go Garbage Collector* — "Go has a non-moving GC." — https://go.dev/doc/gc-guide [1]
2. Wikipedia, *ABA problem* — https://en.wikipedia.org/wiki/ABA_problem [2]
3. Go Standard Library, `weak` package documentation — https://pkg.go.dev/weak [3]
4. Michael Knyszek, *From unique to cleanups and weak: new low-level tools for efficiency*, The Go Blog (March 2025) — https://go.dev/blog/cleanups-and-weak [5]
5. Go Standard Library, `runtime.AddCleanup` documentation — https://pkg.go.dev/runtime#AddCleanup [6]
6. Go Team, *Go 1.25 Release Notes* — "Cleanup functions scheduled by AddCleanup are now executed concurrently and in parallel..." — https://go.dev/doc/go1.25 [7]
7. Go Team, *A Guide to the Go Garbage Collector*, "Common weak pointer issues" — https://go.dev/doc/gc-guide#Finalizers_cleanups_and_weak_pointers [9]
8. Go Issue #67552, *weak: new package providing weak pointers* — https://github.com/golang/go/issues/67552 [10]
9. Go Team, *Go 1.24 Release Notes* — Introduction of `weak` and `runtime.AddCleanup` — https://go.dev/doc/go1.24 [12]
