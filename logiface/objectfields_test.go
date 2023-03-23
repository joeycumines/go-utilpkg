package logiface

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

var (
	// compile time assertions

	_ Event = (*objectFields[Event, Parent[Event]])(nil)
)

// Demonstrates the default object field formats, if the object support is defaulted.
func ExampleObjectBuilder_defaultFieldFormats() {
	type E = *mockSimpleEvent
	var logger *Logger[E] = mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout, MultiLine: true, JSON: true}),
	)

	logger.Info().
		Object().Field(`a`, time.Second*1).Dur(`b`, time.Second*3).Dur(`c`, time.Second/8).Field(`d`, time.Duration(float64(time.Hour)*3.51459)).As(`durs`).End().
		Object().Str(`e`, `str val`).As(`str`).End().
		Object().Bool(`f`, true).Bool(`F`, false).As(`bools`).End().
		Object().Any(`g`, map[int]float64{1: 1.0, 2: 1.0 / 2.0, 3: 1.0 / 3.0}).As(`map`).End().
		Object().Err(errors.New(`some error`)).As(`err`).End().
		Object().Int(`h`, 123).As(`int`).End().
		Object().Uint64(`i`, 123).As(`uint64`).End().
		Object().Int64(`j`, 123).As(`int64`).End().
		Object().Float32(`k`, 1.0/3.0).As(`float32`).End().
		Object().Float64(`l`, 1.0/3.0).As(`float64`).End().
		Object().Time(`m`, time.Unix(0, 0).UTC()).As(`time`).End().
		Object().Base64(`n`, []byte(`valasrdijasdu8jasidjasd`), nil).As(`base64`).End().
		Log(``)

	//output:
	//[info]
	//durs={"a":"1s","b":"3s","c":"0.125s","d":"12652.524s"}
	//str={"e":"str val"}
	//bools={"F":false,"f":true}
	//map={"g":{"1":1,"2":0.5,"3":0.3333333333333333}}
	//err={"err":{}}
	//int={"h":123}
	//uint64={"i":"123"}
	//int64={"j":"123"}
	//float32={"k":0.33333334}
	//float64={"l":0.3333333333333333}
	//time={"m":"1970-01-01T00:00:00Z"}
	//base64={"n":"dmFsYXNyZGlqYXNkdThqYXNpZGphc2Q="}
}

func TestObjectFields_mustEmbedUnimplementedEvent(t *testing.T) {
	(*objectFields[Event, Parent[Event]])(nil).mustEmbedUnimplementedEvent()
}

func TestObjectFields_AddMessage(t *testing.T) {
	defer func() {
		if r := fmt.Sprint(recover()); r != `unimplemented` {
			t.Error(r)
		}
	}()
	(*objectFields[Event, Parent[Event]])(nil).AddMessage(`asdads`)
	t.Error(`expected panic`)
}

func TestObjectFields_Level_nil(t *testing.T) {
	if v := (*objectFields[Event, *Builder[Event]])(nil).Level(); v != LevelDisabled {
		t.Error(v)
	}
}
