package export

import (
	"github.com/go-test/deep"
	"golang.org/x/exp/slices"
	"math/rand"
	"strings"
	"testing"
)

func Test_insertSort(t *testing.T) {
	var values []int
	a := func(value int, expected ...int) {
		t.Helper()
		values = insertSort(values, value)
		if diff := deep.Equal(values, expected); diff != nil {
			t.Fatalf("unexpected value: %#v\n%s", values, strings.Join(diff, "\n"))
		}
	}
	a(1, 1)
	a(1, 1)
	a(-1, -1, 1)
	a(123, -1, 1, 123)
	a(0, -1, 0, 1, 123)
	a(1, -1, 0, 1, 123)
	a(53, -1, 0, 1, 53, 123)
}

func Test_insertSortFunc(t *testing.T) {
	var values []int
	a := func(value int, expected ...int) {
		t.Helper()
		values = insertSortFunc(values, value, func(a, b int) int { return a - b })
		if diff := deep.Equal(values, expected); diff != nil {
			t.Fatalf("unexpected value: %#v\n%s", values, strings.Join(diff, "\n"))
		}
	}
	a(1, 1)
	a(1, 1)
	a(-1, -1, 1)
	a(123, -1, 1, 123)
	a(0, -1, 0, 1, 123)
	a(1, -1, 0, 1, 123)
	a(53, -1, 0, 1, 53, 123)
}

func Test_lessTables(t *testing.T) {
	expect := func(input, expected []Table) {
		t.Helper()
		actual := callOn(slices.Clone(input), func(v []Table) { slices.SortFunc(v, lessCmp(lessTables)) })
		if diff := deep.Equal(actual, expected); diff != nil {
			t.Errorf("unexpected sort value: %#v\n%s", actual, strings.Join(diff, "\n"))
		}
	}
	expect(
		[]Table{
			{`a`, `c`},
			{`c`, `a`},
			{`c`, `c`},
			{``, `z`},
			{`c`, `b`},
			{`a`, `b`},
			{`a`, `a`},
		},
		[]Table{
			{``, `z`},
			{`a`, `a`},
			{`a`, `b`},
			{`a`, `c`},
			{`c`, `a`},
			{`c`, `b`},
			{`c`, `c`},
		},
	)
}

func Test_searchTables(t *testing.T) {
	tables := []Table{
		{``, `z`},
		{`a`, `a`},
		{`a`, `b`},
		{`a`, `c`},
		{`c`, `a`},
		{`c`, `b`},
		{`c`, `c`},
	}
	expect := func(i int, table Table) {
		t.Helper()
		if v := leftResult(slices.BinarySearchFunc(tables, table, lessCmp(lessTables))); v != i {
			t.Error(i, table, v)
		}
	}
	for i, table := range tables {
		expect(i, table)
	}
	expect(0, Table{``, ``})
	expect(0, Table{``, `y`})
	expect(0, Table{``, `z`})
	expect(1, Table{``, string([]byte{'z' + 1})})
	expect(4, Table{`c`, string([]byte{'a' - 1})})
	expect(7, Table{`c`, `d`})
}

func Test_dependencyCycle(t *testing.T) {
	for _, tc := range [...]struct {
		Name  string
		Deps  map[int][]int
		Cycle bool
	}{
		{
			Name: `empty`,
		},
		{
			Name: `no cycle`,
			Deps: map[int][]int{
				1: {2},
				2: {3, 4, 5, 6},
				3: {7},
				8: {3, 4, 6},
				6: {9},
			},
		},
		{
			Name: `has cycle`,
			Deps: map[int][]int{
				1: {2},
				2: {3, 4, 5, 6},
				3: {7},
				8: {3, 4, 6},
				6: {8},
			},
			Cycle: true,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			if dependencyCycle(tc.Deps) != tc.Cycle {
				t.Error(!tc.Cycle)
			}
		})
	}
}

func Test_lessCmp_gt(t *testing.T) {
	if v := lessCmp(func(a, b int) bool {
		if a != 2 || b != 1 {
			t.Fatal(a, b)
		}
		return false
	})(2, 1); v != 1 {
		t.Error(v)
	}
}

func Test_compare2(t *testing.T) {
	values := [][2]int{
		{24, 44},
		{-1, 99},
		{-55, 2},
		{3, 3},
		{6, 2},
		{24, 44},
		{-55, 42},
	}
	expected := [][2]int{
		{-55, 2},
		{-55, 42},
		{-1, 99},
		{3, 3},
		{6, 2},
		{24, 44},
		{24, 44},
	}
	less := cmpLess(func(a, b [2]int) int { return compare2(a[0], a[1], b[0], b[1]) })
	slices.SortFunc(values, lessCmp(less))
	if diff := deep.Equal(values, expected); diff != nil {
		t.Fatalf("unexpected value: %#v\n%s", values, strings.Join(diff, "\n"))
	}
	rnd := rand.New(rand.NewSource(9235344))
	for i := 0; i < 100; i++ {
		rnd.Shuffle(len(values), func(i, j int) { values[i], values[j] = values[j], values[i] })
		slices.SortFunc(values, lessCmp(less))
		if diff := deep.Equal(values, expected); diff != nil {
			t.Fatalf("unexpected value: %#v\n%s", values, strings.Join(diff, "\n"))
		}
	}
}
