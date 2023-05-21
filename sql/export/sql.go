package export

import (
	"errors"
	"fmt"
)

type (
	// Snippet models a SQL snippet + associated args.
	Snippet struct {
		SQL  string
		Args []any
	}

	// Dialect models an SQL dialect.
	// Note that all methods should return (nil, nil) or (nil, ErrUnimplemented) if args is nil.
	// See also UnimplementedDialect.
	Dialect interface {
		SelectBatch(args *SelectBatch) (*Snippet, error)
		SelectRows(args *SelectRows) (*Snippet, error)
		InsertRows(args *InsertRows) (*Snippet, error)

		mustEmbedUnimplementedDialect()
	}

	UnimplementedDialect struct{}

	SelectBatch struct {
		Schema              *Schema
		Offset              map[string]int64
		Filters             []*Snippet
		Limit               uint64
		MaxOffsetConditions int
	}

	SelectRows struct {
		Schema *Schema
		Table  Table
		IDs    []int64
	}

	InsertRows struct {
		Schema  *Schema
		Table   Table
		Columns []string
		Values  []any
	}
)

var (
	ErrUnimplemented = errors.New(`go-sql/export: unimplemented`)

	_ Dialect = UnimplementedDialect{}
)

func (UnimplementedDialect) SelectBatch(*SelectBatch) (*Snippet, error) {
	return nil, fmt.Errorf(`select batch error: %w`, ErrUnimplemented)
}

func (UnimplementedDialect) SelectRows(*SelectRows) (*Snippet, error) {
	return nil, fmt.Errorf(`select rows error: %w`, ErrUnimplemented)
}

func (UnimplementedDialect) InsertRows(*InsertRows) (*Snippet, error) {
	return nil, fmt.Errorf(`insert rows error: %w`, ErrUnimplemented)
}

func (UnimplementedDialect) mustEmbedUnimplementedDialect() {}
