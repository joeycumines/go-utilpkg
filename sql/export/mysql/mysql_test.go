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
	schemaExample1                      = `../../testdata/schema-example-1.json`
	schemaExample1Query                 = "SELECT `bs`.`_id` AS `bs`,`ap`.`_id` AS `ap`,`d`.`_id` AS `d`,`da`.`_id` AS `da`,`df`.`_id` AS `df`,`o`.`_id` AS `o`,`orf`.`_id` AS `orf`,`sm`.`_id` AS `sm`,`w`.`_id` AS `w`,`wa`.`_id` AS `wa`,`wf`.`_id` AS `wf` FROM (((((((((`big_show` AS `bs` LEFT JOIN `angryPandas` AS `ap` ON `bs`.`angryPandaId`=`ap`.`_id`) LEFT JOIN `drongos` AS `d` ON `bs`.`drongo_id`=`d`.`_id`) LEFT JOIN `drongo_arrivals` AS `da` ON `d`.`drongo_arrival`=`da`.`_id`) LEFT JOIN `foods` AS `df` ON `d`.`_id`=`df`.`drongo_id`) LEFT JOIN `outtakes` AS `o` ON `bs`.`outtake_id`=`o`.`_id`) LEFT JOIN `foods` AS `orf` ON `o`.`_id`=`orf`.`outtake_id`) LEFT JOIN `smallMongooses` AS `sm` ON `bs`.`smallMongooseId`=`sm`.`_id`) LEFT JOIN `wheelbarrows` AS `w` ON `bs`.`wheelbarrow_id`=`w`.`_id`) LEFT JOIN `wide_aunties` AS `wa` ON `w`.`wide_auntie`=`wa`.`_id`) LEFT JOIN `foods` AS `wf` ON `w`.`_id`=`wf`.`wheelbarrow_id` WHERE (bs.owo = ?) AND (`bs`.`_id` > ? OR (`bs`.`_id` <=> ? AND (`ap`.`_id` > ? OR (`ap`.`_id` <=> ? AND (`d`.`_id` > ? OR (`d`.`_id` <=> ? AND (`da`.`_id` > ? OR (`da`.`_id` <=> ? AND (`df`.`_id` > ? OR (`df`.`_id` <=> ? AND (`o`.`_id` > ? OR (`o`.`_id` <=> ? AND (`orf`.`_id` > ? OR (`orf`.`_id` <=> ? AND (`sm`.`_id` > ? OR (`sm`.`_id` <=> ? AND (`w`.`_id` > ? OR (`w`.`_id` <=> ? AND (`wa`.`_id` > ? OR (`wa`.`_id` <=> ? AND (`wf`.`_id` > ?))))))))))))))))))))) AND ((CASE WHEN sm.type IS NOT NULL THEN sm.type = 'LARGE' ELSE 1 END)) ORDER BY `bs`.`_id`,`ap`.`_id`,`d`.`_id`,`da`.`_id`,`df`.`_id`,`o`.`_id`,`orf`.`_id`,`sm`.`_id`,`w`.`_id`,`wa`.`_id`,`wf`.`_id` LIMIT 466"
	schemaExample1QueryWhereNoSpaceship = "WHERE (bs.owo = ?) AND (`bs`.`_id` > ? OR ((`bs`.`_id` = ? OR (? IS NULL AND `bs`.`_id` IS NULL)) AND (`ap`.`_id` > ? OR ((`ap`.`_id` = ? OR (? IS NULL AND `ap`.`_id` IS NULL)) AND (`d`.`_id` > ? OR ((`d`.`_id` = ? OR (? IS NULL AND `d`.`_id` IS NULL)) AND (`da`.`_id` > ? OR ((`da`.`_id` = ? OR (? IS NULL AND `da`.`_id` IS NULL)) AND (`df`.`_id` > ? OR ((`df`.`_id` = ? OR (? IS NULL AND `df`.`_id` IS NULL)) AND (`o`.`_id` > ? OR ((`o`.`_id` = ? OR (? IS NULL AND `o`.`_id` IS NULL)) AND (`orf`.`_id` > ? OR ((`orf`.`_id` = ? OR (? IS NULL AND `orf`.`_id` IS NULL)) AND (`sm`.`_id` > ? OR ((`sm`.`_id` = ? OR (? IS NULL AND `sm`.`_id` IS NULL)) AND (`w`.`_id` > ? OR ((`w`.`_id` = ? OR (? IS NULL AND `w`.`_id` IS NULL)) AND (`wa`.`_id` > ? OR ((`wa`.`_id` = ? OR (? IS NULL AND `wa`.`_id` IS NULL)) AND (`wf`.`_id` > ?))))))))))))))))))))) AND ((CASE WHEN sm.type IS NOT NULL THEN sm.type = 'LARGE' ELSE 1 END))"
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
			Name:    `table name only with spaceship`,
			Dialect: &Dialect{NullSafeEqual: true},
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
		{
			Name:    `table name only with spaceship no offset`,
			Dialect: &Dialect{NullSafeEqual: true},
			Args: &export.SelectBatch{
				Schema:  jsonUnmarshalTestResource(schemaExample1, new(export.Schema)),
				Filters: []*export.Snippet{{SQL: `bs.owo = ?`, Args: []any{321}}},
				Limit:   466,
			},
			Snippet: export.Snippet{
				SQL:  "SELECT `bs`.`_id` AS `bs`,`ap`.`_id` AS `ap`,`d`.`_id` AS `d`,`da`.`_id` AS `da`,`df`.`_id` AS `df`,`o`.`_id` AS `o`,`orf`.`_id` AS `orf`,`sm`.`_id` AS `sm`,`w`.`_id` AS `w`,`wa`.`_id` AS `wa`,`wf`.`_id` AS `wf` FROM (((((((((`big_show` AS `bs` LEFT JOIN `angryPandas` AS `ap` ON `bs`.`angryPandaId`=`ap`.`_id`) LEFT JOIN `drongos` AS `d` ON `bs`.`drongo_id`=`d`.`_id`) LEFT JOIN `drongo_arrivals` AS `da` ON `d`.`drongo_arrival`=`da`.`_id`) LEFT JOIN `foods` AS `df` ON `d`.`_id`=`df`.`drongo_id`) LEFT JOIN `outtakes` AS `o` ON `bs`.`outtake_id`=`o`.`_id`) LEFT JOIN `foods` AS `orf` ON `o`.`_id`=`orf`.`outtake_id`) LEFT JOIN `smallMongooses` AS `sm` ON `bs`.`smallMongooseId`=`sm`.`_id`) LEFT JOIN `wheelbarrows` AS `w` ON `bs`.`wheelbarrow_id`=`w`.`_id`) LEFT JOIN `wide_aunties` AS `wa` ON `w`.`wide_auntie`=`wa`.`_id`) LEFT JOIN `foods` AS `wf` ON `w`.`_id`=`wf`.`wheelbarrow_id` WHERE (bs.owo = ?) AND ((CASE WHEN sm.type IS NOT NULL THEN sm.type = 'LARGE' ELSE 1 END)) ORDER BY `bs`.`_id`,`ap`.`_id`,`d`.`_id`,`da`.`_id`,`df`.`_id`,`o`.`_id`,`orf`.`_id`,`sm`.`_id`,`w`.`_id`,`wa`.`_id`,`wf`.`_id` LIMIT 466",
				Args: []any{321},
			},
		},
		{
			Name:    `table name only with spaceship and offset all null`,
			Dialect: &Dialect{NullSafeEqual: true},
			Args: &export.SelectBatch{
				Schema:  jsonUnmarshalTestResource(schemaExample1, new(export.Schema)),
				Filters: []*export.Snippet{{SQL: `bs.owo = ?`, Args: []any{321}}},
				Offset:  map[string]int64{},
				Limit:   466,
			},
			Snippet: export.Snippet{
				SQL:  schemaExample1Query,
				Args: []interface{}{321, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}},
			},
		},
		{
			Name:    `table name only without spaceship`,
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
				SQL:  schemaExample1Query[:887] + schemaExample1QueryWhereNoSpaceship + schemaExample1Query[1424:],
				Args: []interface{}{321, sql.NullInt64{Int64: 42, Valid: true}, sql.NullInt64{Int64: 42, Valid: true}, sql.NullInt64{Int64: 42, Valid: true}, sql.NullInt64{Int64: 6, Valid: true}, sql.NullInt64{Int64: 6, Valid: true}, sql.NullInt64{Int64: 6, Valid: true}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}, sql.NullInt64{Int64: 0, Valid: false}},
			},
		},
		{
			Name:    `simple with table schema`,
			Dialect: (*Dialect)(nil),
			Args: &export.SelectBatch{
				Schema: func() *export.Schema {
					v, err := (&export.Template{Targets: map[string]*export.Target{`t`: {
						Table:      export.Table{Schema: `schm`, Name: `tbl`},
						PrimaryKey: `somePrimaryKey`,
					}}}).Schema()
					if err != nil {
						t.Fatal(err)
					}
					return v
				}(),
			},
			Snippet: export.Snippet{SQL: "SELECT `t`.`somePrimaryKey` AS `t` FROM `schm`.`tbl` AS `t` ORDER BY `t`.`somePrimaryKey`"},
		},
		{
			Name:    `preserves name case`,
			Dialect: (*Dialect)(nil),
			Args: &export.SelectBatch{
				Schema: func() *export.Schema {
					v, err := (&export.Template{Targets: map[string]*export.Target{`T_A`: {
						Table:      export.Table{Schema: `TBL_SCHEMA`, Name: `TBL_NAME`},
						PrimaryKey: `SOME_PK`,
					}}}).Schema()
					if err != nil {
						t.Fatal(err)
					}
					return v
				}(),
			},
			Snippet: export.Snippet{SQL: "SELECT `T_A`.`SOME_PK` AS `T_A` FROM `TBL_SCHEMA`.`TBL_NAME` AS `T_A` ORDER BY `T_A`.`SOME_PK`"},
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
