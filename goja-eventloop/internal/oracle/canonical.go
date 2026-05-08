package oracle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
)

const maxDifferences = 128

var missingJSON = json.RawMessage(`{"$missing":true}`)

func canonicalJSON(data []byte) (json.RawMessage, any, error) {
	if err := rejectDuplicateKeys(data); err != nil {
		return nil, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, nil, errors.New("trailing JSON value")
		}
		return nil, nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	return encoded, value, nil
}

func compareJSON(want, got []byte) ([]Difference, json.RawMessage, json.RawMessage, error) {
	canonicalWant, wantValue, err := canonicalJSON(want)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("want observation: %w", err)
	}
	canonicalGot, gotValue, err := canonicalJSON(got)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("got observation: %w", err)
	}
	var differences []Difference
	diffValue("$", wantValue, gotValue, &differences)
	return differences, canonicalWant, canonicalGot, nil
}

func diffValue(path string, want, got any, differences *[]Difference) {
	if len(*differences) >= maxDifferences {
		return
	}
	wantMap, wantMapOK := want.(map[string]any)
	gotMap, gotMapOK := got.(map[string]any)
	if wantMapOK && gotMapOK {
		keys := make([]string, 0, len(wantMap)+len(gotMap))
		seen := make(map[string]bool, len(wantMap)+len(gotMap))
		for key := range wantMap {
			keys = append(keys, key)
			seen[key] = true
		}
		for key := range gotMap {
			if !seen[key] {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		for _, key := range keys {
			wantChild, wantOK := wantMap[key]
			gotChild, gotOK := gotMap[key]
			childPath := path + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			switch {
			case !wantOK:
				appendDifference(childPath, missingJSON, marshalRaw(gotChild), differences)
			case !gotOK:
				appendDifference(childPath, marshalRaw(wantChild), missingJSON, differences)
			default:
				diffValue(childPath, wantChild, gotChild, differences)
			}
		}
		return
	}
	wantArray, wantArrayOK := want.([]any)
	gotArray, gotArrayOK := got.([]any)
	if wantArrayOK && gotArrayOK {
		length := max(len(wantArray), len(gotArray))
		for index := range length {
			childPath := fmt.Sprintf("%s/%d", path, index)
			switch {
			case index >= len(wantArray):
				appendDifference(childPath, missingJSON, marshalRaw(gotArray[index]), differences)
			case index >= len(gotArray):
				appendDifference(childPath, marshalRaw(wantArray[index]), missingJSON, differences)
			default:
				diffValue(childPath, wantArray[index], gotArray[index], differences)
			}
		}
		return
	}
	if !reflect.DeepEqual(want, got) {
		appendDifference(path, marshalRaw(want), marshalRaw(got), differences)
	}
}

func appendDifference(path string, want, got json.RawMessage, differences *[]Difference) {
	if len(*differences) >= maxDifferences {
		return
	}
	*differences = append(*differences, Difference{Path: path, Want: want, Got: got})
}

func marshalRaw(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`"<unencodable>"`)
	}
	return data
}
