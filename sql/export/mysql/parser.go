package mysql

import (
	"errors"
	"fmt"
	"github.com/joeycumines/go-utilpkg/sql/export"
	"github.com/pingcap/tidb/parser"
	"github.com/pingcap/tidb/parser/ast"
	"github.com/pingcap/tidb/parser/model"
	"github.com/pingcap/tidb/parser/opcode"
)

type (
	Parser struct {
		Dialect *Dialect

		// Parser is optional, note they aren't thread safe.
		Parser *parser.Parser

		// internal state, used solely by unexported methods

		template         *export.Template
		aliasTables      map[string]export.Table
		aliasPrimaryKeys map[string]string
		aliasForeignKeys map[string]*export.JoinRef
		aliasOrder       map[string]int
		filters          []*export.Snippet
	}
)

// ParseTemplate build an export template by parsing a query string.
// Selected fields must be all primary keys, one for each joined table, potentially more than one per table (multiple
// joins, note that composite keys are unsupported). Selected fields must be like "SELECT alias.pk as alias". If
// provided, ORDER BY must specify primary keys only, at most once per key (per join), and only in ascending order. An
// arbitrary WHERE may be provided, which will be combined (using AND) into the batch select.
func (x Parser) ParseTemplate(query string) (*export.Template, error) {
	if x.Parser == nil {
		x.Parser = parser.New()
	}

	if stmt, err := x.Parser.ParseOneStmt(
		query,
		x.Dialect.charset(),
		x.Dialect.collation(),
	); err != nil {
		return nil, err
	} else if err := x.parseTemplate(stmt); err != nil {
		return nil, err
	}

	return x.template, nil
}

func (x *Parser) parseTemplate(stmt ast.StmtNode) error {
	query, ok := stmt.(*ast.SelectStmt)
	if !ok {
		return fmt.Errorf(`invalid statement type: %T`, stmt)
	}

	if query == nil ||
		query.Distinct ||
		query.GroupBy != nil ||
		query.Having != nil ||
		query.WindowSpecs != nil ||
		query.LockInfo != nil ||
		query.IsInBraces ||
		query.WithBeforeBraces ||
		query.QueryBlockOffset != 0 ||
		query.SelectIntoOpt != nil ||
		query.AfterSetOperator != nil ||
		query.Kind != ast.SelectStmtKindSelect ||
		query.Lists != nil ||
		query.With != nil ||
		query.AsViewSchema {
		return errors.New(`invalid select statement`)
	}

	if err := x.parseAliasTables(query); err != nil {
		return err
	}

	if err := x.parseAliasPrimaryKeys(query); err != nil {
		return err
	}

	if err := x.parseAliasForeignKeys(query); err != nil {
		return err
	}

	if err := x.parseOrder(query); err != nil {
		return err
	}

	if err := x.parseFilters(query); err != nil {
		return err
	}

	x.buildTemplate()

	return nil
}

func (x *Parser) buildTemplate() {
	if len(x.aliasTables) == 0 ||
		len(x.aliasTables) != len(x.aliasPrimaryKeys) ||
		len(x.aliasTables) != len(x.aliasForeignKeys)+1 {
		panic(`unexpected state`)
	}

	template := export.Template{
		Targets: make(map[string]*export.Target, len(x.aliasTables)),
		Filters: x.filters,
	}

	for alias, table := range x.aliasTables {
		template.Targets[alias] = &export.Target{
			Table:      table,
			Order:      x.aliasOrder[alias],
			PrimaryKey: x.aliasPrimaryKeys[alias],
			ForeignKey: x.aliasForeignKeys[alias],
		}
	}

	x.template = &template
}

func (x *Parser) parseAliasTables(query *ast.SelectStmt) (err error) {
	if query == nil || query.From == nil {
		return fmt.Errorf(`parseAliasTables: invalid query`)
	}
	m := make(map[string]export.Table)
	query.From.Accept(&astVisitor{fn: func(node ast.Node) (bool, bool) {
		switch node := node.(type) {
		case *ast.TableRefsClause:
		case *ast.Join:
		case *ast.OnCondition:
			// skip children, continue visiting (it's one of the children of joins)
			return true, true
		case *ast.TableSource:
			var (
				table export.Table
				alias string
			)
			table, alias, err = parseTableSource(node, `parseAliasTables: `)
			if err != nil {
				return false, false
			}
			if _, ok := m[alias]; ok {
				err = fmt.Errorf(`parseAliasTables: duplicate alias: %s`, node.AsName.O)
				return false, false
			}
			m[alias] = table
			// skip the child (we already handled it), continue visiting
			return true, true
		default:
			err = fmt.Errorf(`parseAliasTables: unexpected node type: %T`, node)
			return false, false
		}
		// don't skip children, continue visiting
		return false, true
	}})
	if err != nil {
		return err
	}
	if len(m) == 0 {
		return errors.New(`parseAliasTables: no tables selected`)
	}
	x.aliasTables = m
	return nil
}

func (x *Parser) parseAliasPrimaryKeys(query *ast.SelectStmt) error {
	if query == nil || query.Fields == nil || len(query.Fields.Fields) != len(x.aliasTables) {
		return errors.New(`parseAliasPrimaryKeys: invalid query`)
	}
	m := make(map[string]string, len(query.Fields.Fields))
	singleTableAlias := parseSingleTableAlias(query)
	for i, field := range query.Fields.Fields {
		var name *ast.ColumnName
		if field != nil {
			if expr, _ := field.Expr.(*ast.ColumnNameExpr); expr != nil {
				name = expr.Name
			}
		}
		if name == nil ||
			name.Name.O == `` ||
			field.WildCard != nil ||
			field.Auxiliary ||
			field.AuxiliaryColInAgg ||
			field.AuxiliaryColInOrderBy {
			return fmt.Errorf(`parseAliasPrimaryKeys: invalid field at index %d`, i)
		}
		var alias string
		switch {
		case singleTableAlias != ``:
			alias = singleTableAlias
		case field.AsName.O != ``:
			alias = field.AsName.O
		case name.Schema == (model.CIStr{}) && name.Table.O != ``:
			alias = name.Table.O
		default:
			return fmt.Errorf(`parseAliasPrimaryKeys: unable to resolve alias for field at index %d`, i)
		}
		if _, ok := m[alias]; ok {
			return fmt.Errorf(`parseAliasPrimaryKeys: duplicate field at index %d`, i)
		}
		m[alias] = name.Name.O
	}
	x.aliasPrimaryKeys = m
	return nil
}

func (x *Parser) parseAliasForeignKeys(query *ast.SelectStmt) (err error) {
	if query == nil || query.From == nil {
		return errors.New(`parseAliasForeignKeys: invalid query`)
	}

	if parseSingleTableAlias(query) != `` {
		// no joins
		return nil
	}

	x.aliasForeignKeys = make(map[string]*export.JoinRef)

	validateForeignKey := func(table export.Table, column string, refTable export.Table) error {
		// ensure there's no existing-but-different value
		for alias, ref := range x.aliasForeignKeys {
			if ref.Column != column {
				continue
			}
			// ref.Column is for only one of alias and ref.Alias (also, we need to compare the other value)
			var value export.Table
			if ref.Reverse {
				if x.aliasTables[alias] != table {
					continue
				}
				value = x.aliasTables[ref.Alias]
			} else if x.aliasTables[ref.Alias] != table {
				continue
			} else {
				value = x.aliasTables[alias]
			}
			if value != refTable {
				return fmt.Errorf(`parseAliasForeignKeys: table %q column %q references both %q and %q`, table, column, value, refTable)
			}
		}
		return nil
	}

	query.From.Accept(&astVisitor{fn: func(node ast.Node) (bool, bool) {
		switch node := node.(type) {
		case *ast.TableRefsClause:
		case *ast.Join:
			if node == nil || node.Using != nil || node.On == nil || node.On.Expr == nil {
				err = errors.New(`parseAliasForeignKeys: invalid join`)
				return false, false
			}

			var (
				table export.Table
				alias string
			)
			table, alias, err = parseJoinTableSource(node, `parseAliasForeignKeys: `)
			if err != nil {
				return false, false
			}
			if table != x.aliasTables[alias] {
				panic(`invalid alias tables`)
			}
			if x.aliasForeignKeys[alias] != nil {
				panic(`unexpected duplicate alias`)
			}

			// only support pk = fk (for now)
			expr, ok := node.On.Expr.(*ast.BinaryOperationExpr)
			if !ok {
				err = fmt.Errorf(`parseAliasForeignKeys: unexpected join condition type: %T`, node.On.Expr)
				return false, false
			}
			if expr == nil {
				err = fmt.Errorf(`parseAliasForeignKeys: nil join condition expr`)
				return false, false
			}
			if expr.Op != opcode.EQ {
				err = fmt.Errorf(`parseAliasForeignKeys: unexpected join condition expr op: %s`, expr.Op)
				return false, false
			}

			var LT, LC, RT, RC string
			LT, LC, err = parseColumnNameExpr(expr.L, `parseAliasForeignKeys: `)
			if err != nil {
				return false, false
			}
			RT, RC, err = parseColumnNameExpr(expr.R, `parseAliasForeignKeys: `)
			if err != nil {
				return false, false
			}

			if LT == RT ||
				x.aliasTables[LT] == x.aliasTables[RT] ||
				(LT != alias && RT != alias) ||
				x.aliasPrimaryKeys[LT] == `` ||
				x.aliasPrimaryKeys[RT] == `` ||
				x.aliasTables[LT] == (export.Table{}) ||
				x.aliasTables[RT] == (export.Table{}) ||
				// exactly one should be a primary key
				(x.aliasPrimaryKeys[LT] != LC && x.aliasPrimaryKeys[RT] != RC) ||
				(x.aliasPrimaryKeys[LT] == LC && x.aliasPrimaryKeys[RT] == RC) {
				err = fmt.Errorf(`parseAliasForeignKeys: invalid column expr identifiers: %q, %q, %q, %q`, LT, LC, RT, RC)
				return false, false
			}

			if x.aliasPrimaryKeys[LT] != LC {
				if err = validateForeignKey(x.aliasTables[LT], LC, x.aliasTables[RT]); err != nil {
					return false, false
				}
			}
			if x.aliasPrimaryKeys[RT] != RC {
				if err = validateForeignKey(x.aliasTables[RT], RC, x.aliasTables[LT]); err != nil {
					return false, false
				}
			}

			var ref export.JoinRef
			if LT == alias {
				ref.Alias = RT
				if x.aliasPrimaryKeys[LT] == LC {
					ref.Column = RC
				} else {
					ref.Column = LC
					ref.Reverse = true
				}
			} else {
				ref.Alias = LT
				if x.aliasPrimaryKeys[LT] == LC {
					ref.Column = RC
					ref.Reverse = true
				} else {
					ref.Column = LC
				}
			}

			x.aliasForeignKeys[alias] = &ref

			// note we need to keep visiting children to process all the joins
		case *ast.OnCondition:
			// skip children, continue visiting
			return true, true
		case *ast.TableSource:
			// skip children, continue visiting
			return true, true
		default:
			err = fmt.Errorf(`parseAliasForeignKeys: unexpected node type: %T`, node)
			return false, false
		}
		// don't skip children, continue visiting
		return false, true
	}})
	if err != nil {
		return err
	}

	return nil
}

func (x *Parser) parseOrder(query *ast.SelectStmt) error {
	if query == nil || (query.OrderBy != nil && query.OrderBy.ForUnion) {
		return errors.New(`parseOrder: invalid query`)
	}
	if query.OrderBy == nil {
		return nil
	}
	x.aliasOrder = make(map[string]int, len(query.OrderBy.Items))
	for i, item := range query.OrderBy.Items {
		if item != nil {
			if cne, ok := item.Expr.(*ast.ColumnNameExpr); ok &&
				cne != nil &&
				cne.Refer == nil &&
				cne.Name != nil &&
				cne.Name.Schema == (model.CIStr{}) &&
				cne.Name.Table.O != `` &&
				cne.Name.Name.O != `` &&
				// validates only known aliases
				x.aliasPrimaryKeys[cne.Name.Table.O] != `` &&
				// validates refers to the primary key
				x.aliasPrimaryKeys[cne.Name.Table.O] == cne.Name.Name.O &&
				// must be in ascending order with nulls first (for pagination purposes)
				!item.Desc && item.NullOrder {
				if _, ok := x.aliasOrder[cne.Name.Table.O]; ok {
					return fmt.Errorf(`parseOrder: duplicate order by item at index %d`, i)
				}
				x.aliasOrder[cne.Name.Table.O] = len(query.OrderBy.Items) - i
				continue
			}
		}
		return fmt.Errorf(`parseOrder: invalid order by item at index %d`, i)
	}
	return nil
}

func (x *Parser) parseFilters(query *ast.SelectStmt) error {
	if query == nil {
		return errors.New(`parseFilters: invalid query`)
	}
	if query.Where == nil {
		return nil
	}
	s, err := x.Dialect.astNodeFormat(query.Where)
	if err != nil {
		return err
	}
	x.filters = append(x.filters, &export.Snippet{SQL: s})
	return nil
}

func parseTableSource(node *ast.TableSource, errPrefix string) (table export.Table, alias string, _ error) {
	if node == nil || node.AsName.O == `` || node.Source == nil {
		return table, alias, fmt.Errorf(`%sinvalid table source or alias`, errPrefix)
	}
	child, ok := node.Source.(*ast.TableName)
	if !ok {
		return table, alias, fmt.Errorf(`%sunexpected table source type: %T`, errPrefix, node.Source)
	}
	if child == nil || child.Name.O == `` {
		return table, alias, fmt.Errorf(`%sinvalid table name`, errPrefix)
	}
	return export.Table{Schema: child.Schema.O, Name: child.Name.O}, node.AsName.O, nil
}

func parseJoinTableSource(node *ast.Join, errPrefix string) (table export.Table, alias string, _ error) {
	var child *ast.TableSource
	if node != nil {
		l, lok := node.Left.(*ast.TableSource)
		r, rok := node.Right.(*ast.TableSource)
		switch {
		case lok && !rok:
			child = l
		case !lok && rok:
			child = r
		case lok && rok:
			if node.Tp == ast.RightJoin {
				child = l
			} else {
				child = r
			}
		}
	}
	if child == nil {
		return table, alias, fmt.Errorf(`%sinvalid join`, errPrefix)
	}
	return parseTableSource(child, errPrefix)
}

func parseColumnNameExpr(expr ast.ExprNode, errPrefix string) (table string, column string, _ error) {
	cne, ok := expr.(*ast.ColumnNameExpr)
	if !ok {
		return ``, ``, fmt.Errorf(`%sunexpected column name expr type: %T`, errPrefix, expr)
	}
	if cne == nil ||
		cne.Name == nil ||
		cne.Name.Schema != (model.CIStr{}) ||
		cne.Name.Table.O == `` ||
		cne.Name.Name.O == `` {
		return ``, ``, fmt.Errorf(`%sinvalid column name expr`, errPrefix)
	}
	return cne.Name.Table.O, cne.Name.Name.O, nil
}

func parseSingleTableAlias(query *ast.SelectStmt) string {
	if query != nil &&
		query.From != nil &&
		query.From.TableRefs != nil &&
		query.From.TableRefs.Right == nil {
		if v, _ := query.From.TableRefs.Left.(*ast.TableSource); v != nil {
			return v.AsName.O
		}
	}
	return ``
}
