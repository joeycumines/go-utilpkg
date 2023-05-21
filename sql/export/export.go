// Package export implements mechanisms to subsets of data, from one database to another.
// Note that it only supports auto-incrementing (single) primary/foreign keys, which must fit in int64.
package export

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/joeycumines/logiface"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	DefaultBatchSize = 1000
)

type (
	Exporter struct {
		Reader Reader
		Writer Writer
		// Mapper maps old ids to new ids.
		Mapper Mapper
		// RowTransformer may be provided as a hook to modify rows before they are inserted.
		RowTransformer RowTransformer
		Schema         *Schema
		Logger         *logiface.Logger[logiface.Event]
		// Offset provides an initial offset, to start querying from.
		Offset map[string]int64
		// Filters further restrict the target data set.
		Filters []*Snippet
		// BatchSize configures the max limit, and defaults to DefaultBatchSize if 0.
		BatchSize int
		// MaxSelectIn configures the maximum number of IDs to "SELECT ... WHERE <id> in (...<values>)".
		// If zero it defaults to the (resolved) batch size.
		MaxSelectIn int
		// MaxOffsetConditions configures the maximum number of offsets columns to support.
		// Default is unlimited.
		MaxOffsetConditions int
	}

	exporterRow struct {
		table   Table
		columns []string
		values  []any
	}

	exporterReaderConfig struct {
		ch        chan<- exporterRow
		tableRows map[Table][]int64
	}

	exporterWriterConfig struct {
		ch <-chan exporterRow
	}
)

func (x *Exporter) Export(ctx context.Context) error {
	if err := x.validate(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// condition(s) that continually reduce the possible result set
	var offset map[string]int64
	if x.Offset != nil {
		offset = make(map[string]int64, len(x.Offset))
		maps.Copy(offset, x.Offset)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		snippet, err := x.Reader.SelectBatch(&SelectBatch{
			Schema:              x.Schema,
			Filters:             x.Filters,
			Offset:              offset,
			Limit:               uint64(x.batchSize()),
			MaxOffsetConditions: x.MaxOffsetConditions,
		})
		if err != nil {
			return fmt.Errorf(`select batch error: %w`, err)
		}

		// fetch a batch which is (potentially) a subset a query in the same shape as the schema's
		// (a batch contains only identifiers that aren't known to exist i.e. aren't in x.Mappings)
		var (
			// exit condition: x.batchSize() <= 0 || batchCount < x.batchSize()
			batchCount int
			// we keep track of the number of excluded rows, that weren't after offset
			// if there were rows, but all were excluded, we error (as we were unable to increment offset)
			excludedCount int
			tableRows     = make(map[Table][]int64, len(x.Schema.PrimaryKeys))
		)
		if err := func() error {
			x.Logger.Debug().Log(`fetch started`)
			defer x.Logger.Debug().Log(`fetch stopped`)

			rows, err := x.Reader.QueryContext(ctx, snippet.SQL, snippet.Args...)
			if err != nil {
				return fmt.Errorf(`query error: %w`, err)
			}
			defer rows.Close()

			snippet = nil

			columns, err := rows.Columns()
			if err != nil {
				return err
			}

			// sanity check columns + prep
			columnOrder, ok := x.Schema.columnOrder(columns)
			if !ok {
				return fmt.Errorf(`query error: unexpected columns: %q`, columns)
			}

			// will be used to scan in each row
			// (all columns should be bigint primary keys)
			values := make([]any, len(columns))
			for i := range values {
				values[i] = new(sql.NullInt64)
			}

			for rows.Next() {
				batchCount++
				if err := rows.Scan(values...); err != nil {
					return fmt.Errorf(`scan error: %w`, err)
				}

				// exclude rows that aren't after offset
				// (due to max offset conditions, the db may return rows which should have been excluded)
				if cmpRow(offset, columns, columnOrder, values) <= 0 {
					excludedCount++
					continue
				}

				// determine any rows we need to export
				for i, column := range columns {
					if value := values[i].(*sql.NullInt64); value.Valid {
						table := x.Schema.Template.aliasTable(column)
						_, ok, err := x.Mapper.Load(ctx, table, value.Int64)
						if err != nil {
							return err
						}
						if !ok {
							tableRows[table] = insertSort(tableRows[table], value.Int64)
						}
					}
				}
			}

			if err := rows.Close(); err != nil {
				return fmt.Errorf(`rows close error: %w`, err)
			}

			// update offset (so the next batch starts where we left off)
			if offset == nil {
				offset = make(map[string]int64, len(columns))
			}
			for i, column := range columns {
				if value := values[i].(*sql.NullInt64); value.Valid {
					offset[column] = value.Int64
				} else {
					delete(offset, column)
				}
			}

			return nil
		}(); err != nil {
			return err
		}

		// copy all tableRows, from reader to writer, in dependency order
		if err := func() (err error) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// TODO might want to make this buffer size explicitly configurable
			// (if the rows are fat then it may blow up)
			// TODO consider optimisations e.g. pre-allocated buffer pool for columns (used by queued rows)
			ch := make(chan exporterRow, x.batchSize()*3)

			readerCh := x.startReader(ctx, exporterReaderConfig{
				ch:        ch,
				tableRows: tableRows,
			})

			writerCh := x.startWriter(ctx, exporterWriterConfig{
				ch: ch,
			})

			// note: 2x channels, both send exactly once, never closed
			for i := 0; i < 2; i++ {
				var e error
				select {
				case e = <-readerCh:
				case e = <-writerCh:
				}
				if e != nil && err == nil {
					// cancel on first error
					cancel()
					// err = first non-nil e
					err = e
				}
			}

			return
		}(); err != nil {
			return err
		}

		if x.batchSize() <= 0 || batchCount < x.batchSize() {
			// export complete
			return nil
		}

		if excludedCount != 0 {
			logiface.MapObject[logiface.Event](
				x.Logger.Warning().
					Int(`excluded`, excludedCount).
					Int(`batch`, batchCount),
				`offset`,
				offset,
			).Log(`excluded rows not after offset`)
		}

		if excludedCount >= batchCount {
			return errors.New(`query error: unable to increment offset (check max offset conditions)`)
		}
	}
}

func (x *Exporter) validate() error {
	if x == nil {
		return errors.New(`nil exporter`)
	}
	if err := x.Schema.validate(); err != nil {
		return err
	}
	if err := x.validateReader(); err != nil {
		return err
	}
	if err := x.validateWriter(); err != nil {
		return err
	}
	if x.Mapper == nil {
		return errors.New(`nil mapper`)
	}
	return nil
}

func (x *Exporter) validateReader() error {
	if x.Reader == nil {
		return errors.New(`nil reader`)
	}
	if _, err := x.Reader.SelectBatch(nil); err != nil {
		return err
	}
	if _, err := x.Reader.SelectRows(nil); err != nil {
		return err
	}
	return nil
}

func (x *Exporter) validateWriter() error {
	if x.Writer == nil {
		return errors.New(`nil writer`)
	}
	if _, err := x.Writer.InsertRows(nil); err != nil {
		return err
	}
	return nil
}

func (x *Exporter) batchSize() int {
	if x.BatchSize == 0 {
		return DefaultBatchSize
	}
	if x.BatchSize < 0 {
		return 0
	}
	return x.BatchSize
}

func (x *Exporter) maxSelectIn() int {
	if x.MaxSelectIn == 0 {
		return x.batchSize()
	}
	if x.MaxSelectIn < 0 {
		return 0
	}
	return x.MaxSelectIn
}

func (x *Exporter) startReader(ctx context.Context, cfg exporterReaderConfig) <-chan error {
	ch := make(chan error, 1)
	go func() {
		err := errors.New(`reader: unexpected exit`)
		defer func() { ch <- err }()
		err = x.read(ctx, cfg)
	}()
	return ch
}

func (x *Exporter) read(ctx context.Context, cfg exporterReaderConfig) error {
	x.Logger.Debug().Log(`reader started`)
	defer x.Logger.Debug().Log(`reader stopped`)

	// we skip sending rows that reference rows we failed to read
	missingTableRows := make(map[Table][]int64)

	// for stable ordering (not strictly necessary - might remove later)
	tableOrder := callOn(maps.Keys(cfg.tableRows), func(v []Table) { slices.SortFunc(v, lessTables) })

	for len(cfg.tableRows) != 0 {
		table, ok := func() (table Table, ok bool) {
			for k := range cfg.tableRows {
				if !tableDepsMet(x.Schema.Dependencies, cfg.tableRows, k) {
					continue
				}
				if ok && leftResult(slices.BinarySearchFunc(tableOrder, table, lessCmp(lessTables))) <
					leftResult(slices.BinarySearchFunc(tableOrder, k, lessCmp(lessTables))) {
					continue
				}
				table, ok = k, true
			}
			return
		}()
		if !ok {
			return fmt.Errorf(
				`reader error: cyclic dependency: %+v`,
				callOn(maps.Keys(cfg.tableRows), func(v []Table) { slices.SortFunc(v, lessTables) }),
			)
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		queryRows := cfg.tableRows[table]
		if l := x.maxSelectIn(); l > 0 && len(queryRows) > l {
			queryRows, cfg.tableRows[table] = queryRows[:l], queryRows[l:]
		} else {
			delete(cfg.tableRows, table)
		}

		snippet, err := x.Reader.SelectRows(&SelectRows{
			Schema: x.Schema,
			Table:  table,
			IDs:    queryRows,
		})
		if err != nil {
			return err
		}

		if err := func() error {
			rows, err := x.Reader.QueryContext(ctx, snippet.SQL, snippet.Args...)
			if err != nil {
				return fmt.Errorf(`reader error: %w`, err)
			}
			defer rows.Close()

			snippet = nil

			columns, err := rows.Columns()
			if err != nil {
				return err
			}

			primaryKey, foreignKeys, ok := x.Schema.ColumnIndexes(table, columns)
			if !ok {
				return fmt.Errorf(`reader error: table %q invalid columns: %q`, table, columns)
			}

			// keep track of the next expected row (pk value)
			var next int

			for rows.Next() {
				// pk columns will be *int64, fk columns will be *sql.NullInt64, everything else will be *[]byte
				values := make([]any, len(columns))
				values[primaryKey] = new(int64)
				for _, foreignKey := range foreignKeys {
					values[foreignKey] = new(sql.NullInt64)
				}
				for i, v := range values {
					if v == nil {
						values[i] = new([]byte)
					}
				}

				if err := rows.Scan(values...); err != nil {
					return fmt.Errorf(`reader error: %w`, err)
				}

				// TODO sanity checking of the result set primary key ordering

				primaryKey := *(values[primaryKey].(*int64))

				if next >= len(queryRows) || primaryKey < queryRows[next] {
					return fmt.Errorf(`reader error: unexpected %q primary key: %d`, table, primaryKey)
				}

				for next < len(queryRows) && primaryKey > queryRows[next] {
					// queryRows[next] was missed (e.g. it got deleted)
					missingTableRows[table] = insertSort(missingTableRows[table], queryRows[next])
					next++
				}

				if next >= len(queryRows) || primaryKey != queryRows[next] {
					return fmt.Errorf(`reader error: unexpected %q primary key: %d`, table, primaryKey)
				}

				next++

				if func() bool {
					for column, foreignKey := range foreignKeys {
						if v := *(values[foreignKey].(*sql.NullInt64)); v.Valid &&
							rightResult(slices.BinarySearch(missingTableRows[x.Schema.ForeignKeys[table][column]], v.Int64)) {
							return true
						}
					}
					return false
				}() {
					missingTableRows[table] = insertSort(missingTableRows[table], primaryKey)
					continue
				}

				// send the row to the writer
				select {
				case <-ctx.Done():
					return ctx.Err()
				case cfg.ch <- exporterRow{
					table:   table,
					columns: columns,
					values:  values,
				}:
				}
			}

			if err := rows.Close(); err != nil {
				return fmt.Errorf(`reader error: %w`, err)
			}

			for _, missingRow := range queryRows[next:] {
				missingTableRows[table] = insertSort(missingTableRows[table], missingRow)
			}

			return nil
		}(); err != nil {
			return err
		}
	}

	if len(missingTableRows) != 0 {
		if b := x.Logger.Warning(); b.Enabled() {
			tables := maps.Keys(missingTableRows)
			slices.SortFunc(tables, lessTables)
			obj := logiface.Object[logiface.Event](b)
			for _, table := range tables {
				if len(missingTableRows[table]) == 0 {
					continue
				}
				b, err := table.MarshalText()
				if err != nil {
					continue
				}
				arr := logiface.Array[logiface.Event](obj)
				for _, ID := range missingTableRows[table] {
					arr.Int64(ID)
				}
				arr.As(string(b))
			}
			obj.As(`missing`)
			b.Log(`reader missing rows`)
		}
	}

	// tells the writer that we are done
	close(cfg.ch)

	return nil
}

func (x *Exporter) startWriter(ctx context.Context, cfg exporterWriterConfig) <-chan error {
	ch := make(chan error, 1)
	go func() {
		err := errors.New(`writer error: unexpected exit`)
		defer func() { ch <- err }()
		err = x.write(ctx, cfg)
	}()
	return ch
}

func (x *Exporter) write(ctx context.Context, cfg exporterWriterConfig) error {
	x.Logger.Debug().Log(`writer started`)
	defer x.Logger.Debug().Log(`writer stopped`)

	var rowCount int
WriteLoop:
	for {
		var (
			row exporterRow
			ok  bool
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case row, ok = <-cfg.ch:
		}
		if !ok {
			break
		}

		data := Row{
			Schema: x.Schema,
			Table:  row.table,
		}
		{
			i := indexOfValue(row.columns, x.Schema.PrimaryKeys[row.table])
			data.PrimaryKey = *(row.values[i].(*int64))
			data.Columns = append(append(make([]string, 0, len(row.columns)-1), row.columns[:i]...), row.columns[i+1:]...)
			data.Values = append(append(make([]any, 0, len(row.values)-1), row.values[:i]...), row.values[i+1:]...)
		}

		for i, value := range data.Values {
			if table, ok := x.Schema.ForeignKeys[row.table][data.Columns[i]]; ok {
				if value := value.(*sql.NullInt64); value.Valid {
					mapped, ok, err := x.Mapper.Load(ctx, table, value.Int64)
					if err != nil {
						return err
					}
					if !ok {
						// race condition etc...
						x.Logger.Err().
							Str(`table`, table.String()).
							Int64(`id`, value.Int64).
							Str(`fk_table`, row.table.String()).
							Str(`fk_column`, data.Columns[i]).
							Log(`writer missing row`)
						continue WriteLoop
					}
					data.Values[i] = mapped
				} else {
					data.Values[i] = sql.NullInt64{}
				}
			} else {
				data.Values[i] = *(value.(*[]byte))
			}
		}

		if x.RowTransformer != nil {
			if err := x.RowTransformer.TransformRow(ctx, &data); err != nil {
				return err
			}
		}

		snippet, err := x.Writer.InsertRows(&InsertRows{
			Schema:  x.Schema,
			Table:   row.table,
			Columns: data.Columns,
			Values:  data.Values,
		})
		if err != nil {
			return err
		}

		result, err := x.Writer.ExecContext(ctx, snippet.SQL, snippet.Args...)
		if err != nil {
			return fmt.Errorf(`writer error: %w`, err)
		}

		snippet = nil

		rowCount++

		insertedID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf(`writer error: %w`, err)
		}

		if err := x.Mapper.Store(ctx, row.table, data.PrimaryKey, insertedID); err != nil {
			return err
		}
	}

	x.Logger.Info().
		Int(`count`, rowCount).
		Log(`inserted row(s)`)

	return nil
}

func tableDepsMet(deps map[Table][]Table, rows map[Table][]int64, table Table) bool {
	for _, dep := range deps[table] {
		if len(rows[dep]) != 0 {
			return false
		}
	}
	return true
}

// cmdRow returns an integer indicating the relative position (in the order) vs offset
func cmpRow(offset map[string]int64, columns []string, columnOrder []int, values []any) int {
	var (
		o  int64
		ok bool
		v  sql.NullInt64
	)
	for _, i := range columnOrder {
		o, ok = offset[columns[i]]
		v = *(values[i].(*sql.NullInt64))
		// o NULL, v NULL
		if !ok && !v.Valid {
			continue
		}
		// o NULL, v NOT NULL
		if !ok && v.Valid {
			return 1
		}
		// o NOT NULL, v NULL
		if ok && !v.Valid {
			return -1
		}
		// o NOT NULL, v NOT NULL
		if o < v.Int64 {
			return 1
		}
		if o > v.Int64 {
			return -1
		}
	}
	return 0
}
