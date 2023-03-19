package logiface

import (
	"os"
)

var (
	// compile time assertions

	_ Parent[Event]             = (*Context[Event])(nil)
	_ Parent[Event]             = (*Builder[Event])(nil)
	_ Parent[Event]             = (*ArrayBuilder[Event, *Builder[Event]])(nil)
	_ Parent[Event]             = (*Chainable[Event, *Builder[Event]])(nil)
	_ chainableInterface[Event] = (*Chainable[Event, *Builder[Event]])(nil)
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
		Parent().
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
		Parent().
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
