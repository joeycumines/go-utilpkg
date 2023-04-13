package mysql

import (
	"bytes"
	"database/sql"
	"github.com/joeycumines/go-utilpkg/sql/export"
	"github.com/pingcap/tidb/parser/ast"
	"github.com/pingcap/tidb/parser/format"
	"github.com/pingcap/tidb/parser/model"
	"github.com/pingcap/tidb/parser/opcode"
	"strings"
)

const (
	DefaultCharset   = `utf8mb4`
	DefaultCollation = `utf8mb4_bin`
)

type (
	Dialect struct {
		Charset   string
		Collation string

		// NullSafeEqual may be set to true to indicate the availability of the "<=>" operator (null-safe equal to,
		// also known as the spaceship operator).
		NullSafeEqual bool

		//lint:ignore U1000 embedded for it's methods
		unimplementedDialect
	}

	//lint:ignore U1000 used to embed without exporting
	unimplementedDialect = export.UnimplementedDialect
)

var (
	_ export.Dialect = (*Dialect)(nil)
)

func (x *Dialect) charset() string {
	if x == nil || x.Charset == `` {
		return DefaultCharset
	}
	return x.Charset
}

func (x *Dialect) collation() string {
	if x == nil || x.Collation == `` {
		return DefaultCollation
	}
	return x.Collation
}

func (x *Dialect) restoreFlags() format.RestoreFlags { return format.DefaultRestoreFlags }

func (x *Dialect) astNodeFormat(node ast.Node) (string, error) {
	return astNodeFormat(node, x.restoreFlags())
}

// offsetSQL generates an expression that filters (effectively) "where row greater than x"
// like "a > ? OR (a <=> ? AND (b > ? OR (b <=> ? AND (c > ?))))".
// See also offsetArgs, to generate the necessary prepared statement variables.
func (x *Dialect) offsetSQL(a *export.SelectBatch) string {
	var b bytes.Buffer
	c := format.NewRestoreCtx(x.restoreFlags(), &b)
	var (
		depth int // number of trailing ")"
		last  string
	)
	for i, alias := range a.Schema.AliasOrder {
		if a.MaxOffsetConditions > 0 && a.MaxOffsetConditions <= i {
			break
		}
		if last != `` {
			c.WritePlain(` OR (`)
			depth++
			if x.NullSafeEqual {
				c.WritePlain(last)
				c.WritePlain(` <=> ? AND (`)
			} else {
				c.WritePlain(`(`)
				c.WritePlain(last)
				c.WritePlain(` = ? OR (? IS NULL AND `)
				c.WritePlain(last)
				c.WritePlain(` IS NULL)) AND (`)
			}
			depth++
		}
		i := b.Len()
		c.WriteName(alias)
		c.WritePlain(`.`)
		c.WriteName(a.Schema.Template.Targets[alias].PrimaryKey)
		last = string(b.Bytes()[i:])
		c.WritePlain(` > ?`)
	}
	for depth > 0 {
		c.WritePlain(`)`)
		depth--
	}
	return b.String()
}

// offsetArgs appends the value for each PK in the order / frequency matching offsetSQL.
func (x *Dialect) offsetArgs(a *export.SelectBatch, args []any, values map[string]int64) []any {
	var (
		value sql.NullInt64
		ok    bool
	)
	for i, alias := range a.Schema.AliasOrder {
		if a.MaxOffsetConditions > 0 && a.MaxOffsetConditions <= i {
			break
		}
		// all value except the last (in order) is appended 2 or 3 times
		if ok {
			if x.NullSafeEqual {
				args = append(args, value)
			} else {
				args = append(args, value, value)
			}
		} else {
			ok = true
		}
		// if a value isn't in the map it'll be sent as null
		value.Int64, value.Valid = values[alias]
		args = append(args, value)
	}
	return args
}

func (x *Dialect) limit(limit uint64) *ast.Limit {
	if limit <= 0 {
		return nil
	}
	return &ast.Limit{Count: ast.NewValueExpr(limit, x.charset(), x.collation())}
}

func (x *Dialect) SelectBatch(args *export.SelectBatch) (*export.Snippet, error) {
	if args == nil {
		return nil, nil
	}

	stmt := ast.SelectStmt{
		SelectStmtOpts: &ast.SelectStmtOpts{SQLCache: true},
		From:           &ast.TableRefsClause{},
		Fields:         &ast.FieldList{},
		OrderBy:        &ast.OrderByClause{},
		Limit:          x.limit(args.Limit),
	}

	for _, alias := range args.Schema.AliasOrder {
		aliasName := newModelCIStr(alias)
		pkName := newModelCIStr(args.Schema.Template.Targets[alias].PrimaryKey)

		table := &ast.TableSource{
			Source: newTableName(args.Schema.Template.Targets[alias].Table),
			AsName: aliasName,
		}
		switch {
		case stmt.From.TableRefs == nil:
			stmt.From.TableRefs = &ast.Join{
				Left: table,
				Tp:   ast.LeftJoin,
			}
		case stmt.From.TableRefs.Right == nil:
			stmt.From.TableRefs.Right = table
		default:
			stmt.From.TableRefs = &ast.Join{
				Left:  stmt.From.TableRefs,
				Right: table,
				Tp:    ast.LeftJoin,
			}
		}

		if args.Schema.Template.Targets[alias].ForeignKey != nil {
			var l, r model.CIStr
			if args.Schema.Template.Targets[alias].ForeignKey.Reverse {
				l = newModelCIStr(args.Schema.Template.Targets[args.Schema.Template.Targets[alias].ForeignKey.Alias].PrimaryKey)
				r = newModelCIStr(args.Schema.Template.Targets[alias].ForeignKey.Column)
			} else {
				l = newModelCIStr(args.Schema.Template.Targets[alias].ForeignKey.Column)
				r = pkName
			}
			stmt.From.TableRefs.On = &ast.OnCondition{Expr: &ast.BinaryOperationExpr{
				Op: opcode.EQ,
				L: &ast.ColumnNameExpr{Name: &ast.ColumnName{
					Table: newModelCIStr(args.Schema.Template.Targets[alias].ForeignKey.Alias),
					Name:  l,
				}},
				R: &ast.ColumnNameExpr{Name: &ast.ColumnName{
					Table: aliasName,
					Name:  r,
				}},
			}}
		}

		stmt.Fields.Fields = append(stmt.Fields.Fields, &ast.SelectField{
			Expr: &ast.ColumnNameExpr{Name: &ast.ColumnName{
				Table: aliasName,
				Name:  pkName,
			}},
			AsName: aliasName,
		})

		stmt.OrderBy.Items = append(stmt.OrderBy.Items, &ast.ByItem{
			Expr: &ast.ColumnNameExpr{Name: &ast.ColumnName{
				Table: aliasName,
				Name:  pkName,
			}},
			NullOrder: true,
		})
	}

	var (
		snippet export.Snippet
		filters []string
	)

	for _, filter := range args.Filters {
		filters = append(filters, filter.SQL)
		snippet.Args = append(snippet.Args, filter.Args...)
	}

	if args.Offset != nil {
		filters = append(filters, x.offsetSQL(args))
		snippet.Args = x.offsetArgs(args, snippet.Args, args.Offset)
	}

	for _, filter := range args.Schema.Template.Filters {
		filters = append(filters, filter.SQL)
		snippet.Args = append(snippet.Args, filter.Args...)
	}

	if len(filters) != 0 {
		var conditions []ast.Node
		for _, content := range filters {
			conditions = append(conditions, &astNodeString{content: content})
		}
		stmt.Where = astNodeAnd(conditions...)
	}

	var err error
	snippet.SQL, err = x.astNodeFormat(&stmt)
	if err != nil {
		return nil, err
	}

	return &snippet, nil
}

func (x *Dialect) SelectRows(args *export.SelectRows) (*export.Snippet, error) {
	if args == nil {
		return nil, nil
	}
	list := make([]ast.ExprNode, 0, len(args.IDs))
	for _, ID := range args.IDs {
		list = append(list, ast.NewValueExpr(ID, x.charset(), x.collation()))
	}
	name := newModelCIStr(args.Schema.PrimaryKeys[args.Table])
	query, err := x.astNodeFormat(&ast.SelectStmt{
		SelectStmtOpts: &ast.SelectStmtOpts{SQLCache: true},
		From:           &ast.TableRefsClause{TableRefs: &ast.Join{Left: &ast.TableSource{Source: newTableName(args.Table)}}},
		Where: &ast.PatternInExpr{
			Expr: &ast.ColumnNameExpr{Name: &ast.ColumnName{Name: name}},
			List: list,
		},
		Fields: &ast.FieldList{Fields: []*ast.SelectField{{WildCard: &ast.WildCardField{}}}},
		OrderBy: &ast.OrderByClause{Items: []*ast.ByItem{{
			Expr:      &ast.ColumnNameExpr{Name: &ast.ColumnName{Name: name}},
			NullOrder: true,
		}}},
	})
	if err != nil {
		return nil, err
	}
	return &export.Snippet{SQL: query}, nil
}

func (x *Dialect) InsertRows(args *export.InsertRows) (*export.Snippet, error) {
	if args == nil {
		return nil, nil
	}
	names := make([]*ast.ColumnName, 0, len(args.Columns))
	values := make([]ast.ExprNode, 0, len(args.Columns))
	for _, column := range args.Columns {
		names = append(names, &ast.ColumnName{Name: newModelCIStr(column)})
		values = append(values, &astExprNodeStub{astNode: &astNodeString{content: `?`}})
	}
	query, err := x.astNodeFormat(&ast.InsertStmt{
		Table:   &ast.TableRefsClause{TableRefs: &ast.Join{Left: &ast.TableSource{Source: newTableName(args.Table)}}},
		Columns: names,
		Lists:   [][]ast.ExprNode{values},
	})
	if err != nil {
		return nil, err
	}
	return &export.Snippet{SQL: query, Args: args.Values}, nil
}

func newModelCIStr(name string) model.CIStr {
	return model.CIStr{
		O: name,
		L: strings.ToLower(name),
	}
}

func newTableName(table export.Table) *ast.TableName {
	return &ast.TableName{
		Schema: newModelCIStr(table.Schema),
		Name:   newModelCIStr(table.Name),
	}
}
