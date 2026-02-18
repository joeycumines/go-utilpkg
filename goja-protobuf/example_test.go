package gojaprotobuf_test

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// exampleDescBytes returns a compiled FileDescriptorSet for examples.
func exampleDescBytes() []byte {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    new("example.proto"),
			Package: new("example"),
			Syntax:  new("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: new("Person"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: new("name"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("name")},
					{Name: new("age"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("age")},
				},
			}},
		}},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		panic(err)
	}
	return data
}

func Example() {
	registry := require.NewRegistry()
	registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())

	rt := goja.New()
	registry.Enable(rt)

	// Make descriptor bytes available to JS
	_ = rt.Set("__desc", rt.NewArrayBuffer(exampleDescBytes()))

	v, err := rt.RunString(`
		const pb = require('protobuf');
		pb.loadDescriptorSet(__desc);
		const Person = pb.messageType('example.Person');
		const p = new Person();
		p.set('name', 'Alice');
		p.set('age', 30);
		p.get('name') + ' is ' + p.get('age');
	`)
	if err != nil {
		panic(err)
	}
	fmt.Println(v.String())
	// Output: Alice is 30
}

func ExampleModule_LoadDescriptorSetBytes() {
	rt := goja.New()
	mod, err := gojaprotobuf.New(rt)
	if err != nil {
		panic(err)
	}

	names, err := mod.LoadDescriptorSetBytes(exampleDescBytes())
	if err != nil {
		panic(err)
	}
	fmt.Println(names)
	// Output: [example.Person]
}

func ExampleModule_WrapMessage() {
	rt := goja.New()
	mod, err := gojaprotobuf.New(rt)
	if err != nil {
		panic(err)
	}
	_, _ = mod.LoadDescriptorSetBytes(exampleDescBytes())

	pb := rt.NewObject()
	mod.SetupExports(pb)
	_ = rt.Set("pb", pb)

	// Create a message in JS and get it in Go
	v, _ := rt.RunString(`
		const Person = pb.messageType('example.Person');
		const p = new Person();
		p.set('name', 'Bob');
		p.set('age', 25);
		p;
	`)

	msg, err := mod.UnwrapMessage(v)
	if err != nil {
		panic(err)
	}
	fmt.Println(msg.ProtoReflect().Descriptor().FullName())

	// Wrap it back for JS
	wrapped := mod.WrapMessage(msg)
	_ = rt.Set("wrapped", wrapped)
	result, _ := rt.RunString(`wrapped.get('name')`)
	fmt.Println(result.String())
	// Output:
	// example.Person
	// Bob
}

// newExampleRuntime creates a runtime with protobuf module and loaded
// example descriptors for use in examples. Returns the runtime and a
// helper that runs JS and returns the result string.
func newExampleRuntime() (*goja.Runtime, func(string) string) {
	rt := goja.New()
	mod, err := gojaprotobuf.New(rt)
	if err != nil {
		panic(err)
	}
	if _, err := mod.LoadDescriptorSetBytes(exampleDescBytes()); err != nil {
		panic(err)
	}
	pb := rt.NewObject()
	mod.SetupExports(pb)
	_ = rt.Set("pb", pb)

	run := func(code string) string {
		v, err := rt.RunString(code)
		if err != nil {
			panic(err)
		}
		return v.String()
	}
	return rt, run
}

func Example_equals() {
	_, run := newExampleRuntime()

	fmt.Println(run(`
		var Person = pb.messageType('example.Person');
		var a = new Person();
		a.set('name', 'Alice');
		a.set('age', 30);

		var b = new Person();
		b.set('name', 'Alice');
		b.set('age', 30);

		var c = new Person();
		c.set('name', 'Bob');
		c.set('age', 25);

		pb.equals(a, b) + ',' + pb.equals(a, c);
	`))
	// Output: true,false
}

func Example_clone() {
	_, run := newExampleRuntime()

	fmt.Println(run(`
		var Person = pb.messageType('example.Person');
		var original = new Person();
		original.set('name', 'Alice');
		original.set('age', 30);

		var copy = pb.clone(original);
		copy.set('name', 'Bob');

		original.get('name') + ',' + copy.get('name');
	`))
	// Output: Alice,Bob
}

func Example_isMessage() {
	_, run := newExampleRuntime()

	fmt.Println(run(`
		var Person = pb.messageType('example.Person');
		var p = new Person();

		pb.isMessage(p) + ',' + pb.isMessage({}) + ',' + pb.isMessage(42);
	`))
	// Output: true,false,false
}

func Example_isFieldSet() {
	_, run := newExampleRuntime()

	fmt.Println(run(`
		var Person = pb.messageType('example.Person');
		var p = new Person();

		var before = pb.isFieldSet(p, 'name');
		p.set('name', 'Alice');
		var after = pb.isFieldSet(p, 'name');

		before + ',' + after;
	`))
	// Output: false,true
}

func Example_clearField() {
	_, run := newExampleRuntime()

	fmt.Println(run(`
		var Person = pb.messageType('example.Person');
		var p = new Person();
		p.set('name', 'Alice');
		p.set('age', 30);

		pb.clearField(p, 'name');
		p.get('name') + ',' + p.get('age');
	`))
	// Output: ,30
}

func Example_timestampRoundTrip() {
	_, run := newExampleRuntime()

	// timestampFromMs creates a Timestamp from milliseconds,
	// timestampMs extracts milliseconds back.
	fmt.Println(run(`
		var ts = pb.timestampFromMs(1700000000000);
		pb.timestampMs(ts);
	`))
	// Output: 1700000000000
}

func Example_durationRoundTrip() {
	_, run := newExampleRuntime()

	// durationFromMs creates a Duration from milliseconds,
	// durationMs extracts milliseconds back.
	fmt.Println(run(`
		var dur = pb.durationFromMs(5500);
		pb.durationMs(dur);
	`))
	// Output: 5500
}

func Example_anyPackUnpack() {
	_, run := newExampleRuntime()

	result := run(`
		var Person = pb.messageType('example.Person');
		var p = new Person();
		p.set('name', 'Alice');
		p.set('age', 30);

		var packed = pb.anyPack(Person, p);
		var unpacked = pb.anyUnpack(packed, Person);
		var match = pb.anyIs(packed, 'example.Person');

		unpacked.get('name') + ',' + match;
	`)
	// anyIs checks type_url which contains the fully-qualified name
	parts := strings.Split(result, ",")
	fmt.Println(parts[0])
	fmt.Println(parts[1])
	// Output:
	// Alice
	// true
}
