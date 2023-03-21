package logiface

import (
	"os"
)

var (
	// compile time assertions

	_ Parent[Event]         = (*Context[Event])(nil)
	_ Parent[Event]         = (*Builder[Event])(nil)
	_ Parent[Event]         = (*ArrayBuilder[Event, *Builder[Event]])(nil)
	_ Parent[Event]         = (*ArrayBuilder[Event, *ArrayBuilder[Event, *Builder[Event]]])(nil)
	_ arrayBuilderInterface = (*ArrayBuilder[Event, *Builder[Event]])(nil)
)

func ExampleArray_nestedArrays() {
	// note: outputs one field per line
	type E = *mockSimpleEvent
	var logger *Logger[E] = newSimpleLogger(os.Stdout, true)

	subLogger := Array[E](Array[E](Array[E](Array[E](Array[E](logger.Clone()).Field(1).Field(true)).Field(2).Field(false).Add()).Field(3)).Field(4)).
		Field(5).
		Add().
		Add().
		Add().
		As(`arr_1`).
		Field(`a`, `A`).
		Logger()

	Array[E](Array[E](Array[E](Array[E](Array[E](subLogger.Notice()).Field(1).Field(true)).Field(2).Field(false).Add()).Field(3)).Field(4)).
		Field(5).
		Add().
		Add().
		Add().
		As(`arr_2`).
		Field(`b`, `B`).
		Log(`msg 1`)

	//output:
	//[notice]
	//arr_1=[1 true [2 false] [3 [4 [5]]]]
	//a=A
	//arr_2=[1 true [2 false] [3 [4 [5]]]]
	//b=B
	//msg=msg 1
}
