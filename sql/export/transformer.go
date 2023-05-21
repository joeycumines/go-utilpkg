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
		Columns    []string
		Values     []any
		PrimaryKey int64
	}
)
