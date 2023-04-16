package export

import (
	"encoding/json"
	"github.com/go-test/deep"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestTable_MarshalJSON_success(t *testing.T) {
	for _, tc := range [...]struct {
		Name  string
		Table Table
		JSON  string
	}{
		{
			Name: `both`,
			Table: Table{
				Schema: `a`,
				Name:   `b`,
			},
			JSON: `"a.b"`,
		},
		{
			Name: `b`,
			Table: Table{
				Name: `b`,
			},
			JSON: `"b"`,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			b, err := json.Marshal(tc.Table)
			if err != nil {
				t.Fatal(err)
			}
			if s := string(b); s != tc.JSON {
				t.Error(s)
			}
		})
	}
}

func TestTable_MarshalJSON_failure(t *testing.T) {
	for _, tc := range [...]struct {
		Name  string
		Table Table
		Error string
	}{
		{
			Name:  `empty`,
			Error: `json: error calling MarshalJSON for type export.Table: invalid table: ""`,
		},
		{
			Name:  `just schema`,
			Table: Table{Schema: `a`},
			Error: `json: error calling MarshalJSON for type export.Table: invalid table: "a."`,
		},
		{
			Name:  `name contains period`,
			Table: Table{Schema: `a`, Name: `b.c`},
			Error: `json: error calling MarshalJSON for type export.Table: invalid table: "a.b.c"`,
		},
		{
			Name:  `schema contains period`,
			Table: Table{Schema: `a.b`, Name: `c`},
			Error: `json: error calling MarshalJSON for type export.Table: invalid table: "a.b.c"`,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			b, err := json.Marshal(tc.Table)
			if err == nil || err.Error() != tc.Error {
				t.Error(err)
			}
			if b != nil {
				t.Error(string(b))
			}
		})
	}
}

func TestTable_UnmarshalJSON_success(t *testing.T) {
	for _, tc := range [...]struct {
		Name  string
		JSON  string
		Table Table
	}{
		{
			Name:  `name`,
			JSON:  `"a"`,
			Table: Table{Name: `a`},
		},
		{
			Name:  `both`,
			JSON:  `"a.b"`,
			Table: Table{Schema: `a`, Name: `b`},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			var table Table
			if err := json.Unmarshal([]byte(tc.JSON), &table); err != nil {
				t.Fatal(err)
			}
			if table != (tc.Table) {
				t.Error(table)
			}
		})
	}
}

func TestTable_UnmarshalJSON_failure(t *testing.T) {
	for _, tc := range [...]struct {
		Name  string
		JSON  string
		Error string
	}{
		{
			Name:  `invalid json`,
			Error: `unexpected end of JSON input`,
		},
		{
			Name:  `empty string`,
			JSON:  `""`,
			Error: `invalid table: ""`,
		},
		{
			Name:  `too many segments`,
			JSON:  `"a.b.c"`,
			Error: `invalid table: "a.b.c"`,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			var table Table
			if err := json.Unmarshal([]byte(tc.JSON), &table); err == nil || err.Error() != tc.Error {
				t.Error(err)
			}
			if table != (Table{}) {
				t.Error(table)
			}
		})
	}
}

func TestTemplate_Schema_success(t *testing.T) {
	for _, tc := range [...]struct {
		Name   string
		Schema *Schema
	}{
		{
			Name:   `example 1`,
			Schema: jsonUnmarshalTestResource(`schema-example-1.json`, new(Schema)),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Schema.Template == nil {
				t.Fatal()
			}
			schema, err := tc.Schema.Template.Schema()
			if err != nil {
				t.Fatal(err)
			}
			if schema.Template != tc.Schema.Template {
				t.Error()
			}
			if diff := deep.Equal(schema, tc.Schema); diff != nil || !reflect.DeepEqual(schema, tc.Schema) {
				t.Errorf("unexpected value: %s\n%s", jsonMarshalToString(schema), strings.Join(diff, "\n"))
			}
		})
	}
}

func TestSchema_dependencyOrder(t *testing.T) {
	rnd := rand.New(rand.NewSource(42314))
	schema := jsonUnmarshalTestResource(`schema-example-1.json`, new(Schema))
	for i := 0; i < 100; i++ {
		rnd.Shuffle(len(schema.AliasOrder), func(i, j int) {
			schema.AliasOrder[i], schema.AliasOrder[j] = schema.AliasOrder[j], schema.AliasOrder[i]
		})
		dependencyOrder(schema.AliasOrder, schema.dependencyOrder)
	}
}

func TestSchema_columnOrder(t *testing.T) {
	for _, tc := range [...]struct {
		Name        string
		AliasOrder  []string
		Columns     []string
		ColumnOrder []int
	}{
		{
			Name:        `success`,
			AliasOrder:  []string{`b`, `c`, `a`},
			Columns:     []string{`a`, `b`, `c`},
			ColumnOrder: []int{1, 2, 0},
		},
		{
			Name:        `single value`,
			AliasOrder:  []string{`B`},
			Columns:     []string{`B`},
			ColumnOrder: []int{0},
		},
		{
			Name:       `mismatched value`,
			AliasOrder: []string{`b`},
			Columns:    []string{`B`},
		},
		{
			Name:       `too few columns`,
			AliasOrder: []string{`b`, `c`, `a`},
			Columns:    []string{`a`, `b`},
		},
		{
			Name:       `too many columns`,
			AliasOrder: []string{`b`, `c`, `a`},
			Columns:    []string{`a`, `b`, `c`, `c`},
		},
		{
			Name: `no values`,
		},
		{
			Name:       `missing value`,
			AliasOrder: []string{`b`, `d`, `a`},
			Columns:    []string{`a`, `b`, `c`},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			indexes, ok := (&Schema{AliasOrder: tc.AliasOrder}).columnOrder(tc.Columns)
			if tc.ColumnOrder == nil {
				if ok {
					t.Error(`expected !ok`)
				}
				return
			}
			if !ok {
				t.Fatal(`expected ok`)
			}
			if diff := deep.Equal(indexes, tc.ColumnOrder); diff != nil {
				t.Errorf("unexpected value: %#v\n%s", indexes, strings.Join(diff, "\n"))
			}
		})
	}
}
