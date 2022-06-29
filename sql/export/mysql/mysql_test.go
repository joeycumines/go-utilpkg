package mysql

import (
	"database/sql"
	"github.com/go-test/deep"
	"github.com/joeycumines/go-sql/export"
	_ "github.com/pingcap/tidb/types/parser_driver"
	"reflect"
	"strings"
	"testing"
)

const (
	schemaExample1      = `../../testdata/schema-example-1.json`
	schemaExample1Query = "SELECT `bs`.`_id` AS `bs`,`ap`.`_id` AS `ap`,`d`.`_id` AS `d`,`da`.`_id` AS `da`,`df`.`_id` AS `df`,`o`.`_id` AS `o`,`orf`.`_id` AS `orf`,`sm`.`_id` AS `sm`,`w`.`_id` AS `w`,`wa`.`_id` AS `wa`,`wf`.`_id` AS `wf` FROM (((((((((`big_show` AS `bs` LEFT JOIN `angryPandas` AS `ap` ON `bs`.`angryPandaId`=`ap`.`_id`) LEFT JOIN `drongos` AS `d` ON `bs`.`drongo_id`=`d`.`_id`) LEFT JOIN `drongo_arrivals` AS `da` ON `d`.`drongo_arrival`=`da`.`_id`) LEFT JOIN `foods` AS `df` ON `d`.`_id`=`df`.`drongo_id`) LEFT JOIN `outtakes` AS `o` ON `bs`.`outtake_id`=`o`.`_id`) LEFT JOIN `foods` AS `orf` ON `o`.`_id`=`orf`.`outtake_id`) LEFT JOIN `smallMongooses` AS `sm` ON `bs`.`smallMongooseId`=`sm`.`_id`) LEFT JOIN `wheelbarrows` AS `w` ON `bs`.`wheelbarrow_id`=`w`.`_id`) LEFT JOIN `wide_aunties` AS `wa` ON `w`.`wide_auntie`=`wa`.`_id`) LEFT JOIN `foods` AS `wf` ON `w`.`_id`=`wf`.`wheelbarrow_id` WHERE (bs.owo = ?) AND (`bs`.`_id` > ? OR (`bs`.`_id` <=> ? AND (`ap`.`_id` > ? OR (`ap`.`_id` <=> ? AND (`d`.`_id` > ? OR (`d`.`_id` <=> ? AND (`da`.`_id` > ? OR (`da`.`_id` <=> ? AND (`df`.`_id` > ? OR (`df`.`_id` <=> ? AND (`o`.`_id` > ? OR (`o`.`_id` <=> ? AND (`orf`.`_id` > ? OR (`orf`.`_id` <=> ? AND (`sm`.`_id` > ? OR (`sm`.`_id` <=> ? AND (`w`.`_id` > ? OR (`w`.`_id` <=> ? AND (`wa`.`_id` > ? OR (`wa`.`_id` <=> ? AND (`wf`.`_id` > ?))))))))))))))))))))) AND ((CASE WHEN sm.type IS NOT NULL THEN sm.type = 'LARGE' ELSE 1 END)) ORDER BY `bs`.`_id`,`ap`.`_id`,`d`.`_id`,`da`.`_id`,`df`.`_id`,`o`.`_id`,`orf`.`_id`,`sm`.`_id`,`w`.`_id`,`wa`.`_id`,`wf`.`_id` LIMIT 466"
)

func TestDialect_charset(t *testing.T) {
	if v := (&Dialect{}).charset(); v != `utf8mb4` {
		t.Error(v)
	}
	if v := (&Dialect{Collation: `a`}).charset(); v != `utf8mb4` {
		t.Error(v)
	}
	if v := (&Dialect{Charset: `a`}).charset(); v != `a` {
		t.Error(v)
	}
}

func TestDialect_collation(t *testing.T) {
	if v := (&Dialect{}).collation(); v != `utf8mb4_bin` {
		t.Error(v)
	}
	if v := (&Dialect{Charset: `a`}).collation(); v != `utf8mb4_bin` {
		t.Error(v)
	}
	if v := (&Dialect{Collation: `a`}).collation(); v != `a` {
		t.Error(v)
	}
}

func TestDialect_SelectBatch_nil(t *testing.T) {
	if v, err := (*Dialect)(nil).SelectBatch(nil); err != nil || v != nil {
		t.Error(v, err)
	}
}

func TestDialect_SelectBatch_success(t *testing.T) {
	for _, tc := range [...]struct {
		Name    string
		Dialect *Dialect
		Args    *export.SelectBatch
		Snippet export.Snippet
	}{
		{
			Name:    `table name only`,
			Dialect: &Dialect{},
			Args: &export.SelectBatch{
				Schema:  jsonUnmarshalTestResource(schemaExample1, new(export.Schema)),
				Filters: []*export.Snippet{{SQL: `bs.owo = ?`, Args: []any{321}}},
				Offset: map[string]int64{
					`bs`: 42,
					`ap`: 6,
				},
				Limit: 466,
			},
			Snippet: export.Snippet{
				SQL:  schemaExample1Query,
				Args: []any{321, sql.NullInt64{Int64: 42, Valid: true}, sql.NullInt64{Int64: 42, Valid: true}, sql.NullInt64{Int64: 6, Valid: true}, sql.NullInt64{Int64: 6, Valid: true}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			snippet, err := tc.Dialect.SelectBatch(tc.Args)
			if err != nil || snippet == nil {
				t.Fatal(snippet, err)
			}
			if snippet.SQL != tc.Snippet.SQL {
				expectTextFormattedSQL(t, snippet.SQL, tc.Snippet.SQL)
			}
			if diff := deep.Equal(snippet.Args, tc.Snippet.Args); diff != nil {
				t.Errorf("unexpected value: %#v\n%s", snippet.Args, strings.Join(diff, "\n"))
			}
		})
	}
}

func TestDialect_SelectRows_nil(t *testing.T) {
	if v, err := (*Dialect)(nil).SelectRows(nil); err != nil || v != nil {
		t.Error(v, err)
	}
}

func TestDialect_SelectRows_success(t *testing.T) {
	for _, tc := range [...]struct {
		Name    string
		Dialect *Dialect
		Args    *export.SelectRows
		SQL     string
	}{
		{
			Name:    `table name only`,
			Dialect: &Dialect{},
			Args: &export.SelectRows{
				Schema: &export.Schema{
					PrimaryKeys: map[export.Table]string{
						{Name: `tAblE_NAmE`}: `ColuMn_naME`,
					},
				},
				Table: export.Table{Name: `tAblE_NAmE`},
				IDs:   []int64{4, 2, 91, -233},
			},
			SQL: "SELECT * FROM `tAblE_NAmE` WHERE `ColuMn_naME` IN (4,2,91,-233) ORDER BY `ColuMn_naME`",
		},
		{
			Name:    `table schema and name`,
			Dialect: &Dialect{},
			Args: &export.SelectRows{
				Schema: &export.Schema{
					PrimaryKeys: map[export.Table]string{
						{Schema: `sChemEa_saD`, Name: `tAblE_NAmE`}: `ColuMn_naME`,
					},
				},
				Table: export.Table{Schema: `sChemEa_saD`, Name: `tAblE_NAmE`},
				IDs:   []int64{4, 2, 91, -233},
			},
			SQL: "SELECT * FROM `sChemEa_saD`.`tAblE_NAmE` WHERE `ColuMn_naME` IN (4,2,91,-233) ORDER BY `ColuMn_naME`",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			snippet, err := tc.Dialect.SelectRows(tc.Args)
			if err != nil || snippet == nil {
				t.Fatal(snippet, err)
			}
			if snippet.SQL != tc.SQL {
				expectTextFormattedSQL(t, snippet.SQL, tc.SQL)
			}
		})
	}
}

func TestDialect_InsertRows_nil(t *testing.T) {
	if v, err := (*Dialect)(nil).InsertRows(nil); err != nil || v != nil {
		t.Error(v, err)
	}
}

func TestDialect_InsertRows_success(t *testing.T) {
	for _, tc := range [...]struct {
		Name    string
		Dialect *Dialect
		Args    *export.InsertRows
		SQL     string
	}{
		{
			Name:    `table name only`,
			Dialect: &Dialect{},
			Args: &export.InsertRows{
				Schema:  &export.Schema{},
				Table:   export.Table{Name: `tAblE_NAmE`},
				Columns: []string{`a`, `B`},
				Values:  []any{1, 2},
			},
			SQL: "INSERT INTO `tAblE_NAmE` (`a`,`B`) VALUES (?,?)",
		},
		{
			Name:    `table schema and name`,
			Dialect: &Dialect{},
			Args: &export.InsertRows{
				Schema:  &export.Schema{},
				Table:   export.Table{Schema: `sChemEa_saD`, Name: `tAblE_NAmE`},
				Columns: []string{`a`, `B`},
				Values:  []any{1, 2},
			},
			SQL: "INSERT INTO `sChemEa_saD`.`tAblE_NAmE` (`a`,`B`) VALUES (?,?)",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			snippet, err := tc.Dialect.InsertRows(tc.Args)
			if err != nil || snippet == nil {
				t.Fatal(snippet, err)
			}
			if snippet.SQL != tc.SQL {
				expectTextFormattedSQL(t, snippet.SQL, tc.SQL)
			}
			if !reflect.DeepEqual(tc.Args.Values, snippet.Args) {
				t.Error()
			}
		})
	}
}
