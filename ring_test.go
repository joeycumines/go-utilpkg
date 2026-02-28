package catrate

import (
	"cmp"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func newRingBufferFrom[E cmp.Ordered](s []E) *ringBuffer[E] {
	// get the next power of 2 >= len(s)
	size := 1
	for size < len(s) {
		size <<= 1
	}
	rb := newRingBuffer[E](size)
	copy(rb.s, s)
	rb.w = uint(len(s))
	return rb
}

func TestNewRingBuffer(t *testing.T) {
	size := 8
	rb := newRingBuffer[int](size)

	// Check that the ring buffer is initialized correctly
	if rb == nil {
		t.Fatalf("expected non-nil ring buffer")
	}
	if size != len(rb.s) {
		t.Errorf("len(s) = %v, want %v", len(rb.s), size)
	}
	if rb.r != 0 {
		t.Errorf("r = %v, want 0", rb.r)
	}
	if rb.w != 0 {
		t.Errorf("w = %v, want 0", rb.w)
	}
}

func TestNewRingBuffer_PanicWithInvalidSize(t *testing.T) {
	assertPanics(t, func() { newRingBuffer[int](0) }, "Expected panic with size 0")
	assertPanics(t, func() { newRingBuffer[int](3) }, "Expected panic with non-power of 2 size")
}

func assertPanics(t *testing.T, f func(), msg string) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s", msg)
		}
	}()
	f()
}

func TestNewRingBufferFrom(t *testing.T) {
	tests := []struct {
		name string
		s    []int
		want *ringBuffer[int]
	}{
		{
			name: "Empty Slice",
			s:    []int{},
			want: &ringBuffer[int]{r: 0, w: 0, s: []int{0}},
		},
		{
			name: "Single Element",
			s:    []int{5},
			want: &ringBuffer[int]{r: 0, w: 1, s: []int{5}},
		},
		{
			name: "Multiple Elements",
			s:    []int{1, 2, 3, 4},
			want: &ringBuffer[int]{r: 0, w: 4, s: []int{1, 2, 3, 4}},
		},
		{
			name: "Not power of 2",
			s:    []int{1, 2, 3, 4, 5},
			want: &ringBuffer[int]{r: 0, w: 5, s: []int{1, 2, 3, 4, 5, 0, 0, 0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newRingBufferFrom(tt.s)
			// Compare the fields of the ring buffers
			if !reflect.DeepEqual(got.r, tt.want.r) {
				t.Errorf("r = %v, want %v", got.r, tt.want.r)
			}
			if !reflect.DeepEqual(got.w, tt.want.w) {
				t.Errorf("w = %v, want %v", got.w, tt.want.w)
			}
			if !reflect.DeepEqual(got.s, tt.want.s) {
				t.Errorf("s = %v, want %v", got.s, tt.want.s)
			}
			if len(got.s) != got.Cap() {
				t.Errorf("len(s) = %v, want %v", len(got.s), got.Cap())
			}
		})
	}
}

func TestRingBuffer_Search(t *testing.T) {
	t.Run("empty ring buffer", func(t *testing.T) {
		rb := newRingBuffer[int](2)
		index := rb.Search(5)
		if index != 0 {
			t.Errorf("Unexpected index returned for empty ring buffer: got %v, want 0", index)
		}
	})

	t.Run("non-empty ring buffer", func(t *testing.T) {
		rb := newRingBufferFrom[int]([]int{1, 3, 5, 7, 9})
		index := rb.Search(5)
		if index != 2 {
			t.Errorf("Unexpected index returned for non-empty ring buffer: got %v, want 2", index)
		}

		index = rb.Search(10)
		if index != 5 {
			t.Errorf("Unexpected index returned for non-empty ring buffer when searching for non-existent element: got %v, want 5", index)
		}
	})

	t.Run("ring buffer with duplicate elements", func(t *testing.T) {
		rb := newRingBufferFrom[int]([]int{1, 2, 2, 3, 4})
		index := rb.Search(2)
		if index != 1 {
			t.Errorf("Unexpected index returned for ring buffer with duplicate elements: got %v, want 1", index)
		}
	})
}

func TestRingBuffer_Insert(t *testing.T) {
	t.Run("insert into an empty ring buffer", func(t *testing.T) {
		rb := newRingBuffer[int](2)
		rb.Insert(0, 5)
		if rb.Len() != 1 {
			t.Errorf("Unexpected size after insert: got %v, want 1", rb.Len())
		}
		if rb.Get(0) != 5 {
			t.Errorf("Unexpected value at index 0 after insert: got %v, want 5", rb.Get(0))
		}
	})

	t.Run("insert into a non-empty ring buffer", func(t *testing.T) {
		rb := newRingBufferFrom[int]([]int{1, 3, 5, 7, 9})
		rb.Insert(2, 2)
		if rb.Len() != 6 {
			t.Errorf("Unexpected size after insert: got %v, want 6", rb.Len())
		}
		if rb.Get(2) != 2 {
			t.Errorf("Unexpected value at index 2 after insert: got %v, want 2", rb.Get(2))
		}
	})

	t.Run("insert into a full ring buffer", func(t *testing.T) {
		rb := newRingBufferFrom[int]([]int{1, 2})
		rb.Insert(1, 3)
		if rb.Len() != 3 {
			t.Errorf("Unexpected size after insert into a full ring buffer: got %v, want 3", rb.Len())
		}
		if rb.Get(1) != 3 {
			t.Errorf("Unexpected value at index 1 after insert into a full ring buffer: got %v, want 3", rb.Get(1))
		}
	})

	t.Run("insert out of range", func(t *testing.T) {
		rb := newRingBufferFrom[int]([]int{1, 2, 3, 4, 5})
		assertPanics(t, func() { rb.Insert(6, 6) }, "The code did not panic")
	})

	t.Run("insert into a wrapped around buffer", func(t *testing.T) {
		newBuffer := func() (*ringBuffer[float64], []float64) {
			rb := newRingBuffer[float64](16)

			// start as "read up", not far from the end
			rb.w = uint(len(rb.s)) - 4
			rb.r = rb.w

			written := make([]float64, 9)
			for i := range written {
				f := float64(i) + 1.1
				written[i] = f
				rb.s[int((rb.w+uint(i))%uint(len(rb.s)))] = f
			}
			rb.w += uint(len(written))
			if rb.Len() != len(written) {
				t.Fatal(rb.Len())
			}
			for i, v := range written {
				vb := rb.Get(i)
				if vb != v {
					t.Fatal(vb, v)
				}
			}
			if !reflect.DeepEqual(written, rb.Slice()) {
				t.Errorf("Slice() = %v, want %v", rb.Slice(), written)
			}

			{
				var v [3]int
				v[0], v[1], v[2] = rb.bounds()
				if v != [3]int{12, 16, 5} {
					t.Errorf("bounds() = %v, want %v", v, [3]int{12, 16, 5})
				}
			}

			return rb, written
		}
		_, written := newBuffer()
		for i := 0; i <= len(written); i++ {
			i := i
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				v := float64(1)

				rb, written := newBuffer()
				rb.Insert(i, v)

				// do the same to written
				written = append(written, 0)
				copy(written[i+1:], written[i:])
				written[i] = v

				if !reflect.DeepEqual(written, rb.Slice()) {
					t.Errorf("Slice() = %v, want %v", rb.Slice(), written)
				}
			})
		}
	})

	t.Run("insert into a buffer that is about to wrap around", func(t *testing.T) {
		newBuffer := func() (*ringBuffer[float64], []float64) {
			rb := newRingBuffer[float64](16)

			written := make([]float64, 5)

			rb.w = uint(len(rb.s) - len(written))
			rb.r = rb.w

			for i := range written {
				f := float64(i) + 1.1
				written[i] = f
				rb.s[int((rb.w+uint(i))%uint(len(rb.s)))] = f
			}

			rb.w += uint(len(written))
			if rb.Len() != len(written) {
				t.Fatal(rb.Len())
			}

			for i, v := range written {
				vb := rb.Get(i)
				if vb != v {
					t.Fatal(vb, v)
				}
			}

			if !reflect.DeepEqual(written, rb.Slice()) {
				t.Errorf("Slice() = %v, want %v", rb.Slice(), written)
			}

			{
				var v [3]int
				v[0], v[1], v[2] = rb.bounds()
				if v != [3]int{11, 16} {
					t.Errorf("bounds() = %v, want %v", v, [3]int{11, 16})
				}
			}

			return rb, written
		}
		_, written := newBuffer()
		for i := 0; i <= len(written); i++ {
			i := i
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				v := float64(1)

				rb, written := newBuffer()
				rb.Insert(i, v)

				// do the same to written
				written = append(written, 0)
				copy(written[i+1:], written[i:])
				written[i] = v

				if !reflect.DeepEqual(written, rb.Slice()) {
					t.Errorf("Slice() = %v, want %v", rb.Slice(), written)
				}
			})
		}
	})
}

func FuzzRingBuffer_Insert(f *testing.F) {
	f.Add(int64(1))
	f.Add(int64(2))
	f.Add(int64(-23434245))
	f.Add(int64(4))

	f.Fuzz(func(t *testing.T, randomSeed int64) {
		// needs to be deterministic
		r := rand.New(rand.NewSource(randomSeed))

		rb := newRingBuffer[int](1 << 8)
		if rb.Len() != 0 {
			t.Fatalf("expected size to be 0, got %d", rb.Len())
		}

		const n = 1 << 12

		expected := make([]int, 0, n)

		var shifted []int

		//var check bool
		for i := range n {
			index := r.Intn(rb.Len() + 1)
			value := r.Int()

			rb.Insert(index, value)

			if rb.Len() != i+1-len(shifted) {
				t.Fatalf("iter[%d]: expected size to be %d, got %d", i, i+1-len(shifted), rb.Len())
			}
			if rb.Get(index) != value {
				t.Fatalf("iter[%d]: expected %d at index %d, got %d", i, value, index, rb.Get(index))
			}

			// do the same to expected...
			expectedIndex := index + len(shifted)
			expected = append(expected, 0)
			copy(expected[expectedIndex+1:], expected[expectedIndex:])
			expected[expectedIndex] = value

			// 5% chance of shifting 1-10 elements
			if r.Intn(20) == 0 {
				shift := min(r.Intn(10)+1, rb.Len())
				// add to shifted from rb
				for j := range shift {
					shifted = append(shifted, rb.Get(j))
				}
				rb.RemoveBefore(shift)
				if rb.Len()+len(shifted) != i+1 {
					t.Fatalf("expected size to be %d, got %d", i+1-len(shifted), rb.Len())
				}
			}
		}

		if len(expected) != len(shifted)+rb.Len() {
			t.Fatalf("expected %d elements, got %d", len(expected), len(shifted)+rb.Len())
		}

		for i, v := range shifted {
			if v != expected[i] {
				t.Fatalf("expected %d at index %d, got %d", expected[i], i, v)
			}
		}

		for i := len(shifted); i < n; i++ {
			if rb.Get(i-len(shifted)) != expected[i] {
				t.Fatalf("expected %d at index %d, got %d", expected[i], i, rb.Get(i-len(shifted)))
			}
		}
	})
}
