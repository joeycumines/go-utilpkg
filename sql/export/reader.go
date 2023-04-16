package export

import (
	"context"
	"database/sql"
	"io"
)

type (
	Reader interface {
		Dialect
		databaseReader[Rows]
	}

	databaseReader[R Rows] interface {
		QueryContext(ctx context.Context, query string, args ...any) (R, error)
	}

	Rows interface {
		Close() error
		Columns() ([]string, error)
		Err() error
		Next() bool
		Scan(dest ...any) error
	}

	ReaderImpl[C databaseReader[R], R Rows] struct {
		Dialect
		DB C
	}
)

var (
	_ Reader = (*ReaderImpl[*sql.DB, *sql.Rows])(nil)
)

func (x *ReaderImpl[C, R]) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return x.DB.QueryContext(ctx, query, args...)
}

func (x *ReaderImpl[C, R]) Close() error {
	if v, ok := any(x.DB).(io.Closer); ok {
		return v.Close()
	}
	return nil
}
