package logiface

import (
	"os"
)

func ExampleObject_nestedObjects() {
	type E = *mockSimpleEvent
	var logger *Logger[E] = mockL.New(
		mockL.WithEventFactory(NewEventFactoryFunc(mockSimpleEventFactory)),
		mockL.WithWriter(&mockSimpleWriter{Writer: os.Stdout, MultiLine: true, JSON: true}),
	)

	subLogger := Object[E](Object[E](Object[E](Object[E](Object[E](logger.Clone()).Field(`a`, 1).Field(`b`, true)).Field(`c`, 2).Field(`d`, false).As(`e`)).Field(`f`, 3)).Field(`g`, 4)).
		Field(`h`, 5).
		As(`i`).
		As(`j`).
		As(`k`).
		As(`arr_1`).
		Field(`l`, `A`).
		Logger()

	Object[E](Object[E](Object[E](Object[E](Object[E](subLogger.Notice()).Field(`m`, 1).Field(`n`, true)).Field(`o`, 2).Field(`p`, false).As(`q`)).Field(`r`, 3)).Field(`s`, 4)).
		Field(`t`, 5).
		As(`u`).
		As(`v`).
		As(`w`).
		As(`arr_2`).
		Field(`x`, `B`).
		Log(`msg 1`)

	//output:
	//[notice]
	//arr_1={"a":1,"b":true,"e":{"c":2,"d":false},"k":{"f":3,"j":{"g":4,"i":{"h":5}}}}
	//l="A"
	//arr_2={"m":1,"n":true,"q":{"o":2,"p":false},"w":{"r":3,"v":{"s":4,"u":{"t":5}}}}
	//x="B"
	//msg="msg 1"
}
