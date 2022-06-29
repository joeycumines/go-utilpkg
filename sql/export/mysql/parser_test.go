package mysql

import (
	"github.com/go-test/deep"
	"github.com/joeycumines/go-sql/export"
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
