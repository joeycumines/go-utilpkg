package logiface

import (
	"os"
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
	//e={"D":3,"a":1,"b":true,"d":[2,{"c":false}]}
	//h=[5,{"f":4},{"g":6}]
	//j="J"
	//msg="msg 1"
}
