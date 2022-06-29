package mysql

import (
	"encoding/json"
	"fmt"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
)

func jsonUnmarshalTestResource[T any](name string, target T) T {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to find caller source")
	}
	file, err := os.Open(path.Join(path.Dir(source), `testdata`, name))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		panic(err)
	}
	return target
}

func unifiedTextDiff(aName, bName, aText, bText string) string {
	return fmt.Sprint(gotextdiff.ToUnified(
		aName,
		bName,
		aText,
		myers.ComputeEdits(span.URIFromPath(aName), aText, bText),
	))
}

func expectTextWithDiffTransform(transform func(s string) string) func(t *testing.T, actual, expected string) {
	return func(t *testing.T, actual, expected string) {
		if actual == expected {
			return
		}
		t.Helper()
		t.Errorf("unexpected value: %q\n%s", actual, unifiedTextDiff(
			`expected`,
			`actual`,
			transform(expected),
			transform(actual),
		))
	}
}

var expectTextFormattedSQL = expectTextWithDiffTransform(func(s string) string {
	// terrible
	s = strings.Join(strings.Split(s, `,`), ",\n")
	for _, block := range [...]string{
		`FROM`,
		`LEFT JOIN`,
		`WHERE`,
		`ORDER BY`,
		`LIMIT`,
	} {
		s = strings.Join(strings.Split(s, ` `+block+` `), "\n "+block+` `)
	}
	return s
})

func jsonMarshalToString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
