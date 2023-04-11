package export

import (
	"context"
)

type (
	RowTransformer interface {
		TransformRow(ctx context.Context, row *Row) error
	}

	Row struct {
		Schema     *Schema
		Table      Table
		PrimaryKey int64
		Columns    []string
		Values     []any
	}
)
