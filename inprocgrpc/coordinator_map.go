package inprocgrpc

// rpcUintMap is a persistent deterministic treap. Reducer transitions copy
// only the logarithmic search path, so prior states remain immutable without
// cloning every outstanding owner/effect/delivery record.
type rpcUintMap[V any] struct {
	root *rpcUintMapNode[V]
	len  int
}

type rpcUintMapNode[V any] struct {
	key      uint64
	priority uint64
	value    V
	left     *rpcUintMapNode[V]
	right    *rpcUintMapNode[V]
}

func (m rpcUintMap[V]) get(key uint64) (V, bool) {
	for node := m.root; node != nil; {
		switch {
		case key < node.key:
			node = node.left
		case key > node.key:
			node = node.right
		default:
			return node.value, true
		}
	}
	var zero V
	return zero, false
}

func (m rpcUintMap[V]) set(key uint64, value V) rpcUintMap[V] {
	var added bool
	m.root, added = rpcUintMapSet(m.root, key, value)
	if added {
		m.len++
	}
	return m
}

func rpcUintMapSet[V any](
	node *rpcUintMapNode[V],
	key uint64,
	value V,
) (*rpcUintMapNode[V], bool) {
	if node == nil {
		return &rpcUintMapNode[V]{
			key:      key,
			priority: rpcUintPriority(key),
			value:    value,
		}, true
	}
	result := *node
	switch {
	case key < node.key:
		var added bool
		result.left, added = rpcUintMapSet(node.left, key, value)
		if result.left.priority < result.priority {
			return rpcUintMapRotateRight(&result), added
		}
		return &result, added
	case key > node.key:
		var added bool
		result.right, added = rpcUintMapSet(node.right, key, value)
		if result.right.priority < result.priority {
			return rpcUintMapRotateLeft(&result), added
		}
		return &result, added
	default:
		result.value = value
		return &result, false
	}
}

func (m rpcUintMap[V]) delete(key uint64) rpcUintMap[V] {
	var removed bool
	m.root, removed = rpcUintMapDelete(m.root, key)
	if removed {
		m.len--
	}
	return m
}

func rpcUintMapDelete[V any](
	node *rpcUintMapNode[V],
	key uint64,
) (*rpcUintMapNode[V], bool) {
	if node == nil {
		return nil, false
	}
	result := *node
	switch {
	case key < node.key:
		var removed bool
		result.left, removed = rpcUintMapDelete(node.left, key)
		return &result, removed
	case key > node.key:
		var removed bool
		result.right, removed = rpcUintMapDelete(node.right, key)
		return &result, removed
	case node.left == nil:
		return node.right, true
	case node.right == nil:
		return node.left, true
	case node.left.priority < node.right.priority:
		rotated := rpcUintMapRotateRight(&result)
		next := *rotated
		next.right, _ = rpcUintMapDelete(rotated.right, key)
		return &next, true
	default:
		rotated := rpcUintMapRotateLeft(&result)
		next := *rotated
		next.left, _ = rpcUintMapDelete(rotated.left, key)
		return &next, true
	}
}

func rpcUintMapRotateLeft[V any](
	node *rpcUintMapNode[V],
) *rpcUintMapNode[V] {
	right := *node.right
	left := *node
	left.right = right.left
	right.left = &left
	return &right
}

func rpcUintMapRotateRight[V any](
	node *rpcUintMapNode[V],
) *rpcUintMapNode[V] {
	left := *node.left
	right := *node
	right.left = left.right
	left.right = &right
	return &left
}

func rpcUintPriority(key uint64) uint64 {
	// SplitMix64 gives stable, well-distributed priorities for monotonic IDs.
	key += 0x9e3779b97f4a7c15
	key = (key ^ (key >> 30)) * 0xbf58476d1ce4e5b9
	key = (key ^ (key >> 27)) * 0x94d049bb133111eb
	return key ^ (key >> 31)
}
