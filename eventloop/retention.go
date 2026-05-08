package eventloop

import "maps"

import "unsafe"

// retainedMicrotaskJobCapacity is the established ordinary working-set choice.
// All scheduler backing-store limits derive the same logical payload budget so
// differently sized entries do not retain radically different byte totals.
const retainedMicrotaskJobCapacity = 1024

const (
	retainedStorageBytes = uintptr(retainedMicrotaskJobCapacity) * unsafe.Sizeof(microtaskJob{})

	retainedFnQueueCapacity     = int(retainedStorageBytes / unsafe.Sizeof((func())(nil)))
	retainedCheckJobCapacity    = int(retainedStorageBytes / unsafe.Sizeof(checkJob{}))
	retainedLoopCommandCapacity = int(retainedStorageBytes / unsafe.Sizeof(loopCommand{}))
	retainedTimerHeapCapacity   = int(retainedStorageBytes / unsafe.Sizeof((*timerList)(nil)))
	retainedRegistryHighWater   = int(retainedStorageBytes / unsafe.Sizeof(struct {
		key   uint64
		value unsafe.Pointer
	}{}))
	retainedRegistryLowWater = retainedRegistryHighWater / 2
)

func resetRetainedSlice[T any](storage []T, limit int) []T {
	clear(storage)
	if cap(storage) > limit {
		return nil
	}
	return storage[:0]
}

func discardSlice[T any](storage []T) []T {
	clear(storage)
	return nil
}

type retainedMapState struct {
	peak      int
	oversized bool
}

func retainedMapStore[K comparable, V any](entries map[K]V, state *retainedMapState, key K, value V) map[K]V {
	if entries == nil {
		entries = make(map[K]V)
	}
	entries[key] = value
	if len(entries) > state.peak {
		state.peak = len(entries)
	}
	if state.peak > retainedRegistryHighWater {
		state.oversized = true
	}
	return entries
}

func retainedMapDelete[K comparable, V any](entries map[K]V, state *retainedMapState, key K) (map[K]V, bool) {
	delete(entries, key)
	return rebuildRetainedMap(entries, state)
}

func rebuildRetainedMap[K comparable, V any](entries map[K]V, state *retainedMapState) (map[K]V, bool) {
	if !state.oversized {
		return entries, false
	}
	shrinkAt := max(retainedRegistryLowWater, state.peak/2)
	if len(entries) > shrinkAt {
		return entries, false
	}
	replacement := make(map[K]V, len(entries))
	maps.Copy(replacement, entries)
	state.peak = len(entries)
	state.oversized = state.peak > retainedRegistryHighWater
	return replacement, true
}

func discardRetainedMap[K comparable, V any](entries map[K]V, state *retainedMapState) map[K]V {
	clear(entries)
	*state = retainedMapState{}
	return nil
}
