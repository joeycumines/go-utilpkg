package export

import (
	cycle "github.com/joeycumines/go-detect-cycle/floyds"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"
)

// insertSort performs an insert sort, skipping any existing values.
func insertSort[S ~[]E, E constraints.Ordered](values S, value E) S {
	if i, ok := slices.BinarySearch(values, value); !ok {
		ok = i != len(values)
		values = append(values, value)
		if ok {
			copy(values[i+1:], values[i:])
			values[i] = value
		}
	}
	return values
}

// insertSortFunc performs an insert sort, skipping any existing values.
func insertSortFunc[S ~[]E, E any](values S, value E, cmp func(a, b E) int) S {
	if i, ok := slices.BinarySearchFunc(values, value, cmp); !ok {
		ok = i != len(values)
		values = append(values, value)
		if ok {
			copy(values[i+1:], values[i:])
			values[i] = value
		}
	}
	return values
}

// callOn can be used like callOn(maps.Keys(someMap), func(v []string) { sort.Strings(v) })
func callOn[T any](v T, f func(v T)) T {
	f(v)
	return v
}

func lessCmp[E comparable](less func(a, b E) bool) func(a, b E) int {
	return func(a, b E) int {
		if a == b {
			return 0
		}
		if less(a, b) {
			return -1
		}
		return 1
	}
}

func indexOfValue[E comparable](s []E, v E) (i int) {
	for ; i < len(s); i++ {
		if s[i] == v {
			return i
		}
	}
	return
}

func lessTables(a, b Table) bool {
	if a.Schema < b.Schema {
		return true
	} else if a.Schema != b.Schema {
		return false
	}
	return a.Name < b.Name
}

func leftIntBool(i int, _ bool) int { return i }

func rightIntBool(_ int, ok bool) bool { return ok }

func dependencyCycle[E comparable](deps map[E][]E) bool {
	var check func(k E, f cycle.BranchingDetector) bool
	check = func(k E, f cycle.BranchingDetector) bool {
		for _, v := range deps[k] {
			if func() bool {
				nf := f.Hare(v)
				defer nf.Clear()
				if !f.Ok() {
					return true
				}
				if check(v, nf) {
					return true
				}
				return false
			}() {
				return true
			}
		}
		return false
	}
	for k := range deps {
		if check(k, cycle.NewBranchingDetector(k, nil)) {
			return true
		}
	}
	return false
}
