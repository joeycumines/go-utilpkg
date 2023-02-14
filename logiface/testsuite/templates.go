package testsuite

import (
	"errors"
	"github.com/joeycumines/go-utilpkg/logiface"
)

var eventTemplates = []func(in logiface.Event) (out Event){
	eventTemplate1,
	eventTemplate2,
	eventTemplate3,
	eventTemplate4,
	eventTemplate5,
	eventTemplate6,
	eventTemplate7,
	eventTemplate8,
	eventTemplate9,
	eventTemplate10,
}

func eventTemplate1(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	in.AddField(`field_1`, 101)
	out.Fields[`field_1`] = 101.0

	in.AddField(`field_2`, true)
	out.Fields[`field_2`] = true

	in.AddField(`field_3`, `string Value`)
	out.Fields[`field_3`] = `string Value`

	if msg := `some message`; in.AddMessage(msg) {
		out.Message = &msg
	}

	if err := `some error`; in.AddError(errors.New(err)) {
		out.Error = &err
	}

	if in.AddString(`string_1`, `some string`) {
		out.Fields[`string_1`] = `some string`
	}

	if in.AddInt(`int_1`, 201) {
		out.Fields[`int_1`] = 201.0
	}

	if in.AddFloat32(`float32_1`, 25.5) {
		out.Fields[`float32_1`] = 25.5
	}

	return
}

func eventTemplate2(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	if msg := `some message`; in.AddMessage(msg) {
		out.Message = &msg
	}

	if err := `some error`; in.AddError(errors.New(err)) {
		out.Error = &err
	}

	in.AddField(`field_1`, 101)
	out.Fields[`field_1`] = 101.0

	in.AddField(`field_2`, true)
	out.Fields[`field_2`] = true

	in.AddField(`field_3`, `string Value`)
	out.Fields[`field_3`] = `string Value`

	return
}

func eventTemplate3(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	in.AddField(`field_1`, 101)
	out.Fields[`field_1`] = 101.0

	in.AddField(`field_2`, true)
	out.Fields[`field_2`] = true

	in.AddField(`field_3`, `string Value`)
	out.Fields[`field_3`] = `string Value`

	if msg := `some message`; in.AddMessage(msg) {
		out.Message = &msg
	}

	if err := `some error`; in.AddError(errors.New(err)) {
		out.Error = &err
	}

	if in.AddFloat32(`float32_1`, 25.5) {
		out.Fields[`float32_1`] = 25.5
	}

	if in.AddString(`string_1`, `some string`) {
		out.Fields[`string_1`] = `some string`
	}

	if in.AddInt(`int_1`, 201) {
		out.Fields[`int_1`] = 201.0
	}

	return
}

func eventTemplate4(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	in.AddField(`field_1`, 101)
	out.Fields[`field_1`] = 101.0

	in.AddField(`field_2`, true)
	out.Fields[`field_2`] = true

	in.AddField(`field_3`, nil)
	out.Fields[`field_3`] = nil

	return
}

func eventTemplate5(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	in.AddField(`field_1`, 101)
	out.Fields[`field_1`] = 101.0

	in.AddField(`field_2`, true)
	out.Fields[`field_2`] = true

	in.AddField(`field_3`, `string Value`)
	out.Fields[`field_3`] = `string Value`

	if msg := `some message`; in.AddMessage(msg) {
		out.Message = &msg
	}

	if err := `some error`; in.AddError(errors.New(err)) {
		out.Error = &err
	}

	if in.AddString(`string_1`, `some string`) {
		out.Fields[`string_1`] = `some string`
	}

	if in.AddInt(`int_1`, 201) {
		out.Fields[`int_1`] = 201.0
	}

	if in.AddFloat32(`float32_1`, 25.5) {
		out.Fields[`float32_1`] = 25.5
	}

	return
}

func eventTemplate6(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	in.AddField(`field_1`, 101)
	out.Fields[`field_1`] = 101.0

	in.AddField(`field_2`, true)
	out.Fields[`field_2`] = true

	in.AddField(`field_3`, `string Value`)
	out.Fields[`field_3`] = `string Value`

	in.AddField(`bool_1`, false)
	out.Fields[`bool_1`] = false

	in.AddField(`int_1`, -100)
	out.Fields[`int_1`] = -100.0

	in.AddField(`float32_1`, -3.7)
	out.Fields[`float32_1`] = -3.7

	return
}

func eventTemplate7(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	in.AddField(`field_1`, 101)
	out.Fields[`field_1`] = 101.0

	in.AddField(`field_2`, true)
	out.Fields[`field_2`] = true

	in.AddField(`field_3`, `string Value`)
	out.Fields[`field_3`] = `string Value`

	if msg := `some message`; in.AddMessage(msg) {
		out.Message = &msg
	}

	in.AddError(nil)

	if in.AddString(`string_1`, `some string`) {
		out.Fields[`string_1`] = `some string`
	}

	if in.AddInt(`int_1`, 201) {
		out.Fields[`int_1`] = 201.0
	}

	if in.AddFloat32(`float32_1`, 25.5) {
		out.Fields[`float32_1`] = 25.5
	}

	return
}

func eventTemplate8(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	in.AddField(`field_1`, -20.0)
	out.Fields[`field_1`] = -20.0

	in.AddField(`field_2`, false)
	out.Fields[`field_2`] = false

	in.AddField(`field_3`, `test value`)
	out.Fields[`field_3`] = `test value`

	if in.AddString(`string_1`, `test string`) {
		out.Fields[`string_1`] = `test string`
	}

	if in.AddInt(`int_1`, 2021) {
		out.Fields[`int_1`] = 2021.0
	}

	if in.AddFloat32(`float32_1`, 3.14) {
		out.Fields[`float32_1`] = 3.14
	}

	return
}

func eventTemplate9(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	if msg := `another message`; in.AddMessage(msg) {
		out.Message = &msg
	}

	if err := `another error`; in.AddError(errors.New(err)) {
		out.Error = &err
	}

	in.AddField(`field_1`, 50)
	out.Fields[`field_1`] = 50.0

	in.AddField(`field_2`, true)
	out.Fields[`field_2`] = true

	in.AddField(`field_3`, `value`)
	out.Fields[`field_3`] = `value`

	return
}

func eventTemplate10(in logiface.Event) (out Event) {
	out.Level = in.Level()
	out.Fields = make(map[string]interface{})

	in.AddField(`field_1`, 200)
	out.Fields[`field_1`] = 200.0

	in.AddField(`field_2`, false)
	out.Fields[`field_2`] = false

	in.AddField(`field_3`, `another value`)
	out.Fields[`field_3`] = `another value`

	if msg := `yet another message`; in.AddMessage(msg) {
		out.Message = &msg
	}

	if err := `yet another error`; in.AddError(errors.New(err)) {
		out.Error = &err
	}

	if in.AddFloat32(`float32_1`, 2.71) {
		out.Fields[`float32_1`] = 2.71
	}

	if in.AddString(`string_1`, `another string`) {
		out.Fields[`string_1`] = `another string`
	}

	if in.AddInt(`int_1`, 555) {
		out.Fields[`int_1`] = 555.0
	}

	return
}
