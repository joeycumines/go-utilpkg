package mysql

import (
	"github.com/go-test/deep"
	"github.com/joeycumines/go-utilpkg/sql/export"
	"strings"
	"testing"
)

func TestParser_ParseTemplate_schemaExample1(t *testing.T) {
	template, err := (Parser{}).ParseTemplate(schemaExample1Query)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(template, jsonUnmarshalTestResource(`schema-example-1-template.json`, new(export.Template))); diff != nil {
		t.Errorf("unexpected value: %s\n%s", jsonMarshalToString(template), strings.Join(diff, "\n"))
	}
	schema, err := template.Schema()
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(schema.AliasOrder, []string{"bs", "ap", "d", "da", "df", "o", "orf", "sm", "w", "wa", "wf"}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.AliasOrder, strings.Join(diff, "\n"))
	}
	if diff := deep.Equal(schema.ForeignKeys, map[export.Table]map[string]export.Table{
		{Name: "big_show"}: {
			"angryPandaId":    {Name: "angryPandas"},
			"drongo_id":       {Name: "drongos"},
			"outtake_id":      {Name: "outtakes"},
			"smallMongooseId": {Name: "smallMongooses"},
			"wheelbarrow_id":  {Name: "wheelbarrows"},
		},
		{Name: "drongos"}: {"drongo_arrival": {Name: "drongo_arrivals"}},
		{Name: "foods"}: {
			"drongo_id":      {Name: "drongos"},
			"outtake_id":     {Name: "outtakes"},
			"wheelbarrow_id": {Name: "wheelbarrows"},
		},
		{Name: "wheelbarrows"}: {"wide_auntie": {Name: "wide_aunties"}},
	}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.ForeignKeys, strings.Join(diff, "\n"))
	}
}

func TestParser_ParseTemplate_simple(t *testing.T) {
	template, err := (Parser{}).ParseTemplate(`select t.somePrimaryKey from tbl t`)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(template, &export.Template{Targets: map[string]*export.Target{`t`: {
		Table:      export.Table{Name: `tbl`},
		PrimaryKey: `somePrimaryKey`,
	}}}); diff != nil {
		t.Errorf("unexpected value: %s\n%s", jsonMarshalToString(template), strings.Join(diff, "\n"))
	}
	schema, err := template.Schema()
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(schema.AliasOrder, []string{`t`}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.AliasOrder, strings.Join(diff, "\n"))
	}
	if diff := deep.Equal(schema.ForeignKeys, map[export.Table]map[string]export.Table{}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.ForeignKeys, strings.Join(diff, "\n"))
	}
}

func TestParser_ParseTemplate_simpleWithSchema(t *testing.T) {
	template, err := (Parser{}).ParseTemplate(`select t.somePrimaryKey from sch_Ma.tbl t`)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(template, &export.Template{Targets: map[string]*export.Target{`t`: {
		Table:      export.Table{Schema: `sch_Ma`, Name: `tbl`},
		PrimaryKey: `somePrimaryKey`,
	}}}); diff != nil {
		t.Errorf("unexpected value: %s\n%s", jsonMarshalToString(template), strings.Join(diff, "\n"))
	}
	schema, err := template.Schema()
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(schema.AliasOrder, []string{`t`}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.AliasOrder, strings.Join(diff, "\n"))
	}
	if diff := deep.Equal(schema.ForeignKeys, map[export.Table]map[string]export.Table{}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.ForeignKeys, strings.Join(diff, "\n"))
	}
}

func TestParser_ParseTemplate_simpleNoFieldAlias(t *testing.T) {
	template, err := (Parser{}).ParseTemplate(`SELECT PK FROM TBL T`)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(template, &export.Template{Targets: map[string]*export.Target{`T`: {
		Table:      export.Table{Name: `TBL`},
		PrimaryKey: `PK`,
	}}}); diff != nil {
		t.Errorf("unexpected value: %s\n%s", jsonMarshalToString(template), strings.Join(diff, "\n"))
	}
	schema, err := template.Schema()
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(schema.AliasOrder, []string{`T`}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.AliasOrder, strings.Join(diff, "\n"))
	}
	if diff := deep.Equal(schema.ForeignKeys, map[export.Table]map[string]export.Table{}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.ForeignKeys, strings.Join(diff, "\n"))
	}
}

func TestParser_ParseTemplate_joinWithoutAsName(t *testing.T) {
	template, err := (Parser{}).ParseTemplate(`select a.b, c.d from e as a left join f as c on c.g = a.b;`)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(template, &export.Template{Targets: map[string]*export.Target{
		`a`: {
			Table:      export.Table{Name: `e`},
			PrimaryKey: `b`,
		},
		`c`: {
			Table:      export.Table{Name: `f`},
			PrimaryKey: `d`,
			ForeignKey: &export.JoinRef{
				Alias:   `a`,
				Column:  `g`,
				Reverse: true,
			},
		},
	}}); diff != nil {
		t.Errorf("unexpected value: %s\n%s", jsonMarshalToString(template), strings.Join(diff, "\n"))
	}
	schema, err := template.Schema()
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(schema.AliasOrder, []string{"a", "c"}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.AliasOrder, strings.Join(diff, "\n"))
	}
	if diff := deep.Equal(schema.ForeignKeys, map[export.Table]map[string]export.Table{
		{Name: `f`}: {`g`: {Name: `e`}},
	}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.ForeignKeys, strings.Join(diff, "\n"))
	}
}

func TestParser_ParseTemplate_joinWithoutAsNameFlipFK(t *testing.T) {
	template, err := (Parser{}).ParseTemplate(`select a.b, c.d from e as a left join f as c on c.d = a.g;`)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(template, &export.Template{Targets: map[string]*export.Target{
		`a`: {
			Table:      export.Table{Name: `e`},
			PrimaryKey: `b`,
		},
		`c`: {
			Table:      export.Table{Name: `f`},
			PrimaryKey: `d`,
			ForeignKey: &export.JoinRef{
				Alias:  `a`,
				Column: `g`,
			},
		},
	}}); diff != nil {
		t.Errorf("unexpected value: %s\n%s", jsonMarshalToString(template), strings.Join(diff, "\n"))
	}
	schema, err := template.Schema()
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(schema.AliasOrder, []string{"a", "c"}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.AliasOrder, strings.Join(diff, "\n"))
	}
	if diff := deep.Equal(schema.ForeignKeys, map[export.Table]map[string]export.Table{
		{Name: `e`}: {`g`: {Name: `f`}},
	}); diff != nil {
		t.Errorf("unexpected value: %#v\n%s", schema.ForeignKeys, strings.Join(diff, "\n"))
	}
}
