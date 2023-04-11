package export

import (
	"database/sql"
	"testing"
)

func Test_cmpRow(t *testing.T) {
	for _, tc := range [...]struct {
		Name        string
		Offset      map[string]int64
		Columns     []string
		ColumnOrder []int
		Values      []any
		Result      int
	}{
		{
			Name: `equal`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: 0,
		},
		{
			Name: `order case 1`,
			Offset: map[string]int64{
				`a`: -11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: -13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: -1,
		},
		{
			Name: `order case 1`,
			Offset: map[string]int64{
				`a`: -11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{2, 1, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: -13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: 1,
		},
		{
			Name: `order case 3`,
			Offset: map[string]int64{
				`a`: -11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{0, 2, 1},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: -13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: 1,
		},
		{
			Name: `order case 4`,
			Offset: map[string]int64{
				`a`: -11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{0, 1, 2},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: -13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: -1,
		},
		{
			Name: `a less`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11 - 1, Valid: true},
			},
			Result: -1,
		},
		{
			Name: `b less`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12 - 1, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: -1,
		},
		{
			Name: `c less`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13 - 1, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: -1,
		},
		{
			Name: `a greater`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11 + 1, Valid: true},
			},
			Result: 1,
		},
		{
			Name: `b greater`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12 + 1, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: 1,
		},
		{
			Name: `c greater`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13 + 1, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: 1,
		},
		{
			Name: `a null`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{},
			},
			Result: -1,
		},
		{
			Name: `b null`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: -1,
		},
		{
			Name: `c null`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: -1,
		},
		{
			Name: `a unset`,
			Offset: map[string]int64{
				`b`: 12,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: 1,
		},
		{
			Name: `b unset`,
			Offset: map[string]int64{
				`a`: 11,
				`c`: 13,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: 1,
		},
		{
			Name: `c unset`,
			Offset: map[string]int64{
				`a`: 11,
				`b`: 12,
			},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12, Valid: true},
				&sql.NullInt64{Int64: 13, Valid: true},
				&sql.NullInt64{Int64: 11, Valid: true},
			},
			Result: 1,
		},
		{
			Name:        `unset and null`,
			Offset:      map[string]int64{},
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{Int64: 12},
				&sql.NullInt64{Int64: 13},
				&sql.NullInt64{Int64: 11},
			},
			Result: 0,
		},
		{
			Name:        `unset and null nil map`,
			Columns:     []string{`b`, `c`, `a`},
			ColumnOrder: []int{1, 2, 0},
			Values: []any{
				&sql.NullInt64{},
				&sql.NullInt64{},
				&sql.NullInt64{},
			},
			Result: 0,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			result := cmpRow(tc.Offset, tc.Columns, tc.ColumnOrder, tc.Values)
			if result != tc.Result {
				t.Error(result)
			}
		})
	}
}
