package catrate

import (
	"golang.org/x/exp/constraints"
	"sort"
)

type ringBuffer[E constraints.Ordered] struct {
	s    []E
	r, w uint
}

func newRingBuffer[E constraints.Ordered](size int) *ringBuffer[E] {
	if size <= 0 || size&(size-1) != 0 {
		panic(`catrate: ring: size must be a power of 2`)
	}
	return &ringBuffer[E]{s: make([]E, size)}
}

func (x *ringBuffer[E]) mask(val uint) uint {
	return val & (uint(len(x.s)) - 1)
}

func (x *ringBuffer[E]) bounds() (i1, l1, l2 int) {
	if x.r == x.w {
		return
	}
	i1 = int(x.mask(x.r))
	l1 = int(x.mask(x.w))
	if l1 <= i1 {
		l2 = l1
		l1 = len(x.s)
	}
	return
}

func (x *ringBuffer[E]) Len() int {
	return int(x.w - x.r)
}

func (x *ringBuffer[E]) Cap() int {
	return len(x.s)
}

func (x *ringBuffer[E]) Get(i int) E {
	if i < 0 || i >= x.Len() {
		panic(`catrate: ring: get: index out of range`)
	}
	return x.s[x.mask(x.r+uint(i))]
}

func (x *ringBuffer[E]) Slice() (b []E) {
	if l := x.Len(); l != 0 {
		b = make([]E, l)
		i1, l1, l2 := x.bounds()
		copy(b, x.s[i1:l1])
		copy(b[l1-i1:], x.s[:l2])
	}
	return b
}

func (x *ringBuffer[E]) RemoveBefore(index int) {
	if index < 0 || index > x.Len() {
		panic(`catrate: ring: remove before: index out of range`)
	}
	x.r += uint(index)
}

func (x *ringBuffer[E]) Search(value E) int {
	return sort.Search(x.Len(), func(i int) bool {
		return x.Get(i) >= value
	})
}

func (x *ringBuffer[E]) Insert(index int, value E) {
	l := x.Len()
	if index < 0 || index > l {
		panic(`catrate: ring: insert: index out of range`)
	}

	if l == len(x.s) {
		// full, special case: requires expanding the buffer
		s := make([]E, uint(len(x.s))<<1)
		if len(s) == 0 {
			panic(`catrate: ring: insert: overflow`)
		}

		// since we're copying the whole thing anyway, we can start at 0
		i1, l1, l2 := x.bounds()
		l = l1 - i1
		if index < l {
			// insert in the first segment
			copy(s, x.s[i1:i1+index])
			s[index] = value
			copy(s[index+1:], x.s[i1+index:l1])
			l++
			copy(s[l:], x.s[:l2])
			l += l2
		} else {
			// insert in the second segment
			copy(s, x.s[i1:l1])
			copy(s[l:], x.s[:index-l])
			s[index] = value
			copy(s[index+1:], x.s[index-l:l2])
			l += l2 + 1
		}

		x.r = 0
		x.w = uint(l)
		x.s = s
		return
	}

	// optimization: everything works nicer if it's not wrapped around
	// so, if we can, pre-emptively reset the offsets to 0
	var i, j int
	if l == 0 {
		x.r = 0
		x.w = 0
	} else {
		i = int(x.mask(x.r))
		j = int(x.mask(x.w))
	}

	// fastest case: not wrapped around, and there's room to write
	if l == 0 || i < j {
		// still need to insert at index...
		copy(x.s[i+index+1:], x.s[i+index:j])
		x.s[i+index] = value
		x.w++
		return
	}

	// slow case that only adjusts one segment: we need to write index to the wrapped-around part (at the start
	// of the buffer), where len(x.s)-i is the length of the first segment (not the wrapped around part)
	if index >= len(x.s)-i {
		// insert in the wrapped-around part
		index -= len(x.s) - i
		copy(x.s[index+1:], x.s[index:j])
		x.s[index] = value
		x.w++
		return
	}

	// slowest case that requires adjusting both segments: we need to write index to the first segment (at the end
	// of the buffer), where j is the length of the second segment (the wrapped around part)
	copy(x.s[1:], x.s[:j])
	x.s[0] = x.s[len(x.s)-1]
	copy(x.s[i+index+1:], x.s[i+index:])
	x.s[i+index] = value
	x.w++
}
