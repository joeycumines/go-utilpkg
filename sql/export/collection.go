package export

import (
	"cmp"

	cycle "github.com/joeycumines/go-detect-cycle/floyds"
	"golang.org/x/exp/slices"
)

// insertSort performs an insert sort, skipping any existing values.
func insertSort[S ~[]E, E cmp.Ordered](values S, value E) S {
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

func compare2[T1 cmp.Ordered, T2 cmp.Ordered](a1 T1, a2 T2, b1 T1, b2 T2) int {
	if v := cmp.Compare(a1, b1); v != 0 {
		return v
	}
	return cmp.Compare(a2, b2)
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

func cmpLess[E any](cmp func(a, b E) int) func(a, b E) bool {
	return func(a, b E) bool { return cmp(a, b) < 0 }
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

func leftResult[L any, R any](l L, _ R) L { return l }

func rightResult[L any, R any](_ L, r R) R { return r }

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

func dependencyOrder[E any](order []E, depends func(a, b E) bool) {
	for i := len(order) - 2; i >= 0; i-- {
		v := order[i]
		j := len(order) - 1
		for ; i < j; j-- {
			if depends(v, order[j]) {
				break
			}
		}
		if i == j {
			continue
		}
		copy(order[i:j], order[i+1:j+1])
		order[j] = v
		i = j
	}
}
