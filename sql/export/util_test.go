package export

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"runtime"
)

func loadTestResource(name string) []byte {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to find caller source")
	}
	file, err := os.Open(path.Join(path.Dir(source), `testdata`, name))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	b, err := ioutil.ReadAll(file)
	if err != nil {
		panic(err)
	}
	return b
}

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

func jsonMarshalToString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
