package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type (
	// Schema defines the structure of the data to be exported, and may be initialised via Template.Schema.
	Schema struct {
		Template     *Template
		PrimaryKeys  map[Table]string
		ForeignKeys  map[Table]map[string]Table
		Dependencies map[Table][]Table
		// AliasOrder contains each key from Template.Targets, sorted by Target.Order + string sort.
		// Target.Order is in descending order (larger number first), and is higher priority than the string sort.
		// It is to be used control evaluation of the "offset" conditions in the SelectBatch WHERE clause, which, by
		// nature, must align with specific ORDER BY expressions.
		AliasOrder []string
	}

	// Template is the input used to generate a Schema, and is based on a query in the format of
	// "SELECT [<pk> as <alias> ...] {FROM/JOIN} [WHERE] [ORDER BY ...]", where Targets are joined tables.
	Template struct {
		// Targets are all joined tables with alias (can include the same table more than once).
		Targets map[string]*Target

		// Filters are SQL snippets to AND together in groups, to filter the batch data set.
		Filters []*Snippet
	}

	Target struct {
		// ForeignKey is optional, and models the join condition for this Target.
		ForeignKey *JoinRef
		Table      Table
		// PrimaryKey is the column of the table's primary key. It's after SELECT and (in some cases) ON.
		PrimaryKey string
		// Order is used to indicate the order of the predicates (expressions in WHERE and ORDER BY).
		// Larger numbers should appear first.
		Order int
	}

	Table struct {
		Schema string
		Name   string
	}

	// JoinRef models part of an expression like "JOIN table b ON a.b_id = b._id"
	// OR
	// "JOIN table a ON a.b_id = b._id".
	JoinRef struct {
		// Alias represents the joined table.
		Alias string

		// Column is the column name, in ONE OF Alias's table OR the parent Target's table, depending on Reverse.
		Column string

		// Reverse is set to true to indicate that Column belongs to the parent Target, rather than Alias.
		Reverse bool
	}
)

func (x *Template) Schema() (*Schema, error) {
	schema := Schema{Template: x}
	if err := schema.init(); err != nil {
		return nil, err
	}
	return &schema, nil
}

func (x *Template) validate() error {
	if x == nil {
		return errors.New(`nil template`)
	}
	if len(x.Targets) == 0 {
		return errors.New(`no targets`)
	}
	return nil
}

func (x *Template) aliasTable(alias string) Table {
	if v := x.Targets[alias]; v != nil {
		return v.Table
	}
	return Table{}
}

func (x *Template) aliasOrderSorter(a, b string) int {
	return compare2(x.Targets[b].Order, a, x.Targets[a].Order, b)
}

func (x *Schema) ColumnIndexes(table Table, columns []string) (primaryKey int, foreignKeys map[string]int, ok bool) {
	foreignKeys = make(map[string]int, len(x.ForeignKeys[table]))
	for i, column := range columns {
		if column == `` {
			continue
		}
		if column == x.PrimaryKeys[table] {
			if ok {
				return 0, nil, false
			}
			ok = true
			primaryKey = i
		} else if _, ok := x.ForeignKeys[table][column]; ok {
			if _, ok := foreignKeys[column]; ok {
				return 0, nil, false
			}
			foreignKeys[column] = i
		}
	}
	if !ok || len(foreignKeys) != len(x.ForeignKeys[table]) {
		return 0, nil, false
	}
	return
}

func (x *Schema) validate() error {
	if x == nil {
		return errors.New(`nil schema`)
	}
	if err := x.Template.validate(); err != nil {
		return err
	}
	return nil
}

func (x *Schema) init() error {
	if err := x.Template.validate(); err != nil {
		return err
	}

	x.PrimaryKeys = make(map[Table]string)
	x.ForeignKeys = make(map[Table]map[string]Table)
	x.AliasOrder = make([]string, 0, len(x.Template.Targets))
	for alias, target := range x.Template.Targets {
		if alias == `` {
			return errors.New(`empty alias`)
		}

		if target == nil ||
			target.Table.Name == `` ||
			target.PrimaryKey == `` ||
			(target.ForeignKey != nil &&
				(target.ForeignKey.Alias == `` ||
					target.ForeignKey.Column == `` ||
					(target.ForeignKey.Reverse && target.ForeignKey.Column == target.PrimaryKey) ||
					x.Template.Targets[target.ForeignKey.Alias] == nil ||
					(!target.ForeignKey.Reverse && target.ForeignKey.Column == x.Template.Targets[target.ForeignKey.Alias].PrimaryKey))) {
			return fmt.Errorf(`invalid target for alias: %s`, alias)
		}

		if primaryKey, ok := x.PrimaryKeys[target.Table]; !ok {
			x.PrimaryKeys[target.Table] = target.PrimaryKey
		} else if primaryKey != target.PrimaryKey {
			return fmt.Errorf(`mismatched primary key for alias: %s`, alias)
		}

		x.AliasOrder = insertSortFunc(x.AliasOrder, alias, x.Template.aliasOrderSorter)

		if target.ForeignKey == nil {
			continue
		}

		table, value := x.Template.Targets[target.ForeignKey.Alias].Table, target.Table
		if target.ForeignKey.Reverse {
			table, value = value, table
		}

		if v, ok := x.ForeignKeys[table][target.ForeignKey.Column]; ok {
			if v != value {
				return fmt.Errorf(`table %s column %s references both %s and %s`, table, target.ForeignKey.Column, v, value)
			}
			return nil
		}

		if x.ForeignKeys[table] == nil {
			x.ForeignKeys[table] = make(map[string]Table)
		}

		x.ForeignKeys[table][target.ForeignKey.Column] = value
	}

	x.Dependencies = make(map[Table][]Table, len(x.ForeignKeys))
	for table, foreignKeys := range x.ForeignKeys {
		for _, dependency := range foreignKeys {
			x.Dependencies[table] = insertSortFunc(x.Dependencies[table], dependency, lessCmp(lessTables))
		}
	}

	if dependencyCycle(x.Dependencies) {
		return fmt.Errorf(`dependency cycle: %+v`, x.Dependencies)
	}

	dependencyOrder(x.AliasOrder, x.dependencyOrder)

	return nil
}

func (x *Schema) dependencyOrder(a, b string) bool {
	return x.Template.Targets[a].ForeignKey != nil &&
		x.Template.Targets[a].ForeignKey.Alias == b
}

// columnOrder returns indexes are of columns/values in x.AliasOrder, validating all aliases exist
func (x *Schema) columnOrder(columns []string) (indexes []int, _ bool) {
	if len(columns) == 0 || len(columns) != len(x.AliasOrder) {
		return nil, false
	}
	indexes = make([]int, len(columns))
	for i, alias := range x.AliasOrder {
		index := indexOfValue(columns, alias)
		if index == len(columns) {
			return nil, false
		}
		indexes[i] = index
	}
	return indexes, true
}

func (x Table) String() string {
	if x.Schema == `` {
		return x.Name
	}
	return x.Schema + `.` + x.Name
}

func (x *Table) UnmarshalText(text []byte) error {
	p := bytes.Split(text, []byte(`.`))
	if len(p) > 2 {
		return fmt.Errorf(`invalid table: %q`, text)
	}
	for _, v := range p {
		if len(v) == 0 {
			return fmt.Errorf(`invalid table: %q`, text)
		}
	}
	if len(p) == 2 {
		x.Schema, x.Name = string(p[0]), string(p[1])
	} else {
		x.Schema, x.Name = ``, string(p[0])
	}
	return nil
}

func (x Table) MarshalText() ([]byte, error) {
	if x.Name == `` || strings.ContainsRune(x.Name, '.') || strings.ContainsRune(x.Schema, '.') {
		return nil, fmt.Errorf(`invalid table: %q`, x)
	}
	return []byte(x.String()), nil
}

func (x *Table) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return x.UnmarshalText([]byte(s))
}

func (x Table) MarshalJSON() ([]byte, error) {
	b, err := x.MarshalText()
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(b))
}
