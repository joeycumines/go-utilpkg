package export

import (
	"context"
)

type (
	Mapper interface {
		Load(ctx context.Context, table Table, src int64) (dst int64, ok bool, err error)
		Store(ctx context.Context, table Table, src, dst int64) error
	}

	SimpleMapper map[Table]map[int64]int64
)

var (
	_ Mapper = SimpleMapper(nil)
)

func (x SimpleMapper) Load(_ context.Context, table Table, src int64) (dst int64, ok bool, _ error) {
	dst, ok = x[table][src]
	return
}

func (x SimpleMapper) Store(_ context.Context, table Table, src, dst int64) error {
	if x[table] == nil {
		x[table] = make(map[int64]int64)
	}
	x[table][src] = dst
	return nil
}
