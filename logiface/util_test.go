package logiface

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"
)

// sortedLineWriterSplitOnSpace scans and sorts each line, where the sort is performed by splitting on space.
func sortedLineWriterSplitOnSpace(writer io.Writer) (io.WriteCloser, <-chan error) {
	r, w := io.Pipe()
	out := make(chan error, 1)
	go func() {
		var err error
		defer func() {
			out <- err
			close(out)
			_ = r.CloseWithError(err)
		}()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			v := strings.Split(scanner.Text(), ` `)
			sort.Strings(v)
			_, err = strings.NewReader(strings.Join(v, ` `) + "\n").WriteTo(writer)
			if err != nil {
				return
			}
		}
		err = scanner.Err()
	}()
	return w, out
}

type jsonKeyValue struct {
	Key   string
	Value any
}

type jsonKeyValueList []jsonKeyValue

func (k jsonKeyValueList) Len() int {
	return len(k)
}

func (k jsonKeyValueList) Swap(i, j int) {
	k[i], k[j] = k[j], k[i]
}

func (k jsonKeyValueList) Less(i, j int) bool {
	return k[i].Key < k[j].Key
}

func sortKeysForJSONData(data any) any {
	switch v := data.(type) {
	case map[string]any:
		keyValuePairs := make(jsonKeyValueList, 0, len(v))
		for k, val := range v {
			keyValuePairs = append(keyValuePairs, jsonKeyValue{Key: k, Value: sortKeysForJSONData(val)})
		}
		sort.Sort(keyValuePairs)
		return keyValuePairs
	case []any:
		for i, e := range v {
			v[i] = sortKeysForJSONData(e)
		}
		return v
	default:
		return data
	}
}

func sortedKeysJSONMarshal(data any) ([]byte, error) {
	var buffer bytes.Buffer

	switch v := data.(type) {
	case jsonKeyValueList:
		buffer.WriteString("{")
		for i, kv := range v {
			if i > 0 {
				buffer.WriteString(",")
			}
			key, err := json.Marshal(kv.Key)
			if err != nil {
				return nil, err
			}
			buffer.Write(key)
			buffer.WriteString(":")
			value, err := sortedKeysJSONMarshal(kv.Value)
			if err != nil {
				return nil, err
			}
			buffer.Write(value)
		}
		buffer.WriteString("}")
	case []any:
		buffer.WriteString("[")
		for i, e := range v {
			if i > 0 {
				buffer.WriteString(",")
			}
			value, err := sortedKeysJSONMarshal(e)
			if err != nil {
				return nil, err
			}
			buffer.Write(value)
		}
		buffer.WriteString("]")

	default:
		return json.Marshal(data)
	}

	return buffer.Bytes(), nil
}

func sortedJSONMarshal(data any) ([]byte, error) {
	return sortedKeysJSONMarshal(sortKeysForJSONData(data))
}

func Test_sortedJSONMarshal_success(t *testing.T) {
	t.Parallel()
	for _, tc := range [...]struct {
		Name   string
		Input  string
		Output string
	}{
		{
			Name:   `nested`,
			Input:  `{"D":3,"a":1,"b":true,"d":[2,{"c":false}]}`,
			Output: `{"D":3,"a":1,"b":true,"d":[2,{"c":false}]}`,
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			var input any
			if err := json.Unmarshal([]byte(tc.Input), &input); err != nil {
				t.Fatal(err)
			}
			output, err := sortedJSONMarshal(input)
			if err != nil {
				t.Fatal(err)
			}
			if string(output) != tc.Output {
				t.Errorf("unexpected output: %q\n%s", output, output)
			}
		})
	}
}
