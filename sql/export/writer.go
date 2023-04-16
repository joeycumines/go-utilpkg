package export

import (
	"context"
	"database/sql"
	"io"
)

type (
	Writer interface {
		Dialect
		databaseWriter[Result]
	}

	databaseWriter[R Result] interface {
		ExecContext(ctx context.Context, query string, args ...any) (R, error)
	}

	Result interface {
		LastInsertId() (int64, error)
		RowsAffected() (int64, error)
	}

	WriterImpl[C databaseWriter[R], R Result] struct {
		Dialect
		DB C
	}
)

var (
	_ Writer = (*WriterImpl[*sql.DB, sql.Result])(nil)
)

func (x *WriterImpl[C, R]) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return x.DB.ExecContext(ctx, query, args...)
}

func (x *WriterImpl[C, R]) Close() error {
	if v, ok := any(x.DB).(io.Closer); ok {
		return v.Close()
	}
	return nil
}
