package logiface

import (
	"bytes"
	"os"
	"testing"
)

var (
	// compile time assertions

	_ Parent[Event]             = (*Context[Event])(nil)
	_ Parent[Event]             = (*Builder[Event])(nil)
	_ Parent[Event]             = (*ArrayBuilder[Event, *Builder[Event]])(nil)
	_ Parent[Event]             = (*Chain[Event, *Builder[Event]])(nil)
	_ chainInterfaceFull[Event] = (*Chain[Event, *Builder[Event]])(nil)
)

func ExampleBuilder_Array_nestedArrays() {
	// note: outputs one field per line
	type E = *mockSimpleEvent
	var logger *Logger[E] = newSimpleLogger(os.Stdout, true)

	logger.Notice().
		Array().
		Field(1).
		Field(true).
		Array().
		Field(2).
		Field(false).
		Add().
		Array().
		Field(3).
		Array().
		Field(4).
		Array().
		Field(5).
		Add().
		Add().
		Add().
		As(`arr`).
		End().
		Field(`b`, `B`).
		Log(`msg 1`)

	//output:
	//[notice]
	//arr=[1 true [2 false] [3 [4 [5]]]]
	//b=B
	//msg=msg 1
}

func ExampleContext_Array_nestedArrays() {
	// note: outputs one field per line
	type E = *mockSimpleEvent
	var logger *Logger[E] = newSimpleLogger(os.Stdout, true)

	logger.Clone().
		Array().
		Field(1).
		Field(true).
		Array().
		Field(2).
		Field(false).
		Add().
		Array().
		Field(3).
		Array().
		Field(4).
		Array().
		Field(5).
		Add().
		Add().
		Add().
		As(`arr`).
		End().
		Field(`b`, `B`).
		Logger().
		Notice().
		Log(`msg 1`)

	//output:
	//[notice]
	//arr=[1 true [2 false] [3 [4 [5]]]]
	//b=B
	//msg=msg 1
}

func ExampleBuilder_Object_nestedObjects() {
	type E = *mockSimpleEvent
	var logger *Logger[E] = mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout, MultiLine: true, JSON: true}),
	)

	logger.Notice().
		Object().
		Field(`a`, 1).
		Field(`b`, true).
		Object().
		Field(`c`, 2).
		Field(`d`, false).
		As(`e`).
		Object().
		Field(`f`, 3).
		Object().
		Field(`g`, 4).
		Object().
		Field(`h`, 5).
		As(`i`).
		As(`j`).
		As(`k`).
		As(`l`).
		End().
		Field(`m`, `B`).
		Log(`msg 1`)

	//output:
	//[notice]
	//l={"a":1,"b":true,"e":{"c":2,"d":false},"k":{"f":3,"j":{"g":4,"i":{"h":5}}}}
	//m="B"
	//msg="msg 1"
}

func ExampleContext_Object_nestedObjects() {
	type E = *mockSimpleEvent
	var logger *Logger[E] = mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout, MultiLine: true, JSON: true}),
	)

	logger.Clone().
		Object().
		Field(`a`, 1).
		Field(`b`, true).
		Object().
		Field(`c`, 2).
		Field(`d`, false).
		As(`e`).
		Object().
		Field(`f`, 3).
		Object().
		Field(`g`, 4).
		Object().
		Field(`h`, 5).
		As(`i`).
		As(`j`).
		As(`k`).
		As(`l`).
		End().
		Field(`m`, `B`).
		Logger().
		Notice().
		Log(`msg 1`)

	//output:
	//[notice]
	//l={"a":1,"b":true,"e":{"c":2,"d":false},"k":{"f":3,"j":{"g":4,"i":{"h":5}}}}
	//m="B"
	//msg="msg 1"
}

func ExampleBuilder_nestedObjectsAndArrays() {
	type E = *mockSimpleEvent
	var logger *Logger[E] = mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout, MultiLine: true, JSON: true}),
		mockL.WithDPanicLevel(LevelEmergency),
	)

	logger.Notice().
		Object().
		Field(`a`, 1).
		Field(`b`, true).
		Array().
		Field(2).
		Object().
		Field(`c`, false).
		Add().
		As(`d`).
		CurObject().
		Field(`D`, 3).
		Object().
		Field(`aa1`, 1).
		Array().
		Array().Add().
		Array().Add().
		CurArray().
		Field(`aaa1`).
		As(`aaa`).
		CurObject().
		Field(`aa2`, 2).
		As(`aa`).
		As(`e`).
		Array().
		Field(5).
		Object().
		Field(`f`, 4).
		Add().
		Object().
		Field(`g`, 6).
		Add().
		As(`h`).
		End().
		Field(`j`, `J`).
		Log(`msg 1`)

	//output:
	//[notice]
	//e={"D":3,"a":1,"aa":{"aa1":1,"aa2":2,"aaa":[[],[],"aaa1"]},"b":true,"d":[2,{"c":false}]}
	//h=[5,{"f":4},{"g":6}]
	//j="J"
	//msg="msg 1"
}

func ExampleContext_nestedObjectsAndArrays() {
	type E = *mockSimpleEvent
	var logger *Logger[E] = mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout, MultiLine: true, JSON: true}),
		mockL.WithDPanicLevel(LevelEmergency),
	)

	logger.Clone().
		Object().
		Field(`a`, 1).
		Field(`b`, true).
		Array().
		Field(2).
		Object().
		Field(`c`, false).
		Add().
		As(`d`).
		CurObject().
		Field(`D`, 3).
		Object().
		Field(`aa1`, 1).
		Array().
		Array().Add().
		Array().Add().
		CurArray().
		Field(`aaa1`).
		As(`aaa`).
		CurObject().
		Field(`aa2`, 2).
		As(`aa`).
		As(`e`).
		Array().
		Field(5).
		Object().
		Field(`f`, 4).
		Add().
		Object().
		Field(`g`, 6).
		Add().
		As(`h`).
		End().
		Field(`j`, `J`).
		Logger().
		Notice().
		Log(`msg 1`)

	//output:
	//[notice]
	//e={"D":3,"a":1,"aa":{"aa1":1,"aa2":2,"aaa":[[],[],"aaa1"]},"b":true,"d":[2,{"c":false}]}
	//h=[5,{"f":4},{"g":6}]
	//j="J"
	//msg="msg 1"
}

func TestLogger_nestedJSONFunc(t *testing.T) {
	var buf bytes.Buffer

	logger := mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: &buf, MultiLine: true, JSON: true}),
		mockL.WithDPanicLevel(LevelEmergency),
	).Logger()

	logger.
		Clone().
		// nil case for Context.ArrayFunc
		ArrayFunc(`a`, nil).
		// nil case for Context.ObjectFunc
		ObjectFunc(`b`, nil).
		Array().
		// nil case for ArrayBuilder.ArrayFunc
		ArrayFunc(nil).
		// nil case for ArrayBuilder.ObjectFunc
		ObjectFunc(nil).
		As(`e`).
		// nil case for Chain.ArrayFunc
		ArrayFunc(`f`, nil).
		// nil case for Chain.ObjectFunc
		ObjectFunc(`g`, nil).
		// Chain.ArrayFunc
		ArrayFunc(`f`, func(b ContextArray) {
			b.
				// nil case for ArrayBuilder.ArrayFunc
				ArrayFunc(nil).
				// nil case for ArrayBuilder.ObjectFunc
				ObjectFunc(nil)
		}).
		// Chain.ObjectFunc
		ObjectFunc(`g`, func(b ContextObject) {
			b.
				// nil case for ArrayBuilder.ArrayFunc
				ArrayFunc(``, nil).
				// nil case for ArrayBuilder.ObjectFunc
				ObjectFunc(``, nil)
		}).
		End().
		Logger().
		Notice().
		// nil case for Builder.ArrayFunc
		ArrayFunc(`c`, nil).
		// nil case for Builder.ObjectFunc
		ObjectFunc(`d`, nil).
		Array().
		// nil case for ArrayBuilder.ArrayFunc
		ArrayFunc(nil).
		// nil case for ArrayBuilder.ObjectFunc
		ObjectFunc(nil).
		As(`e`).
		// nil case for Chain.ArrayFunc
		ArrayFunc(`f`, nil).
		// nil case for Chain.ObjectFunc
		ObjectFunc(`g`, nil).
		// Chain.ArrayFunc
		ArrayFunc(`f`, nil).
		// Chain.ObjectFunc
		ObjectFunc(`g`, nil).
		// Chain.ArrayFunc
		ArrayFunc(`f`, func(b BuilderArray) {
			b.
				// nil case for ArrayBuilder.ArrayFunc
				ArrayFunc(nil).
				// nil case for ArrayBuilder.ObjectFunc
				ObjectFunc(nil)
		}).
		// Chain.ObjectFunc
		ObjectFunc(`g`, func(b BuilderObject) {
			b.
				// nil case for ArrayBuilder.ArrayFunc
				ArrayFunc(``, nil).
				// nil case for ArrayBuilder.ObjectFunc
				ObjectFunc(``, nil)
		}).
		End().
		Log(``)

	if s := buf.String(); s != "[notice]\na=[]\nb={}\ne=[[],{}]\nf=[]\ng={}\nf=[[],{}]\ng={\"\":{}}\nc=[]\nd={}\ne=[[],{}]\nf=[]\ng={}\nf=[]\ng={}\nf=[[],{}]\ng={\"\":{}}\n" {
		t.Errorf("unexpected output: %q\n%s", s, s)
	}
}
