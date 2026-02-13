package gojaprotojson_test

import (
	"fmt"

	"github.com/dop251/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	gojaprotojson "github.com/joeycumines/goja-protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func Example() {
	rt := goja.New()
	pb, err := gojaprotobuf.New(rt)
	if err != nil {
		panic(err)
	}
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("example.proto"),
			Package: proto.String("example"),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: proto.String("Person"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("name"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("name")},
					{Name: proto.String("age"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("age")},
				},
			}},
		}},
	}
	data, _ := proto.Marshal(fds)
	_, _ = pb.LoadDescriptorSetBytes(data)
	pbObj := rt.NewObject()
	pb.SetupExports(pbObj)
	_ = rt.Set("pb", pbObj)
	pj, _ := gojaprotojson.New(rt, gojaprotojson.WithProtobuf(pb))
	pjObj := rt.NewObject()
	pj.SetupExports(pjObj)
	_ = rt.Set("protojson", pjObj)

	v, _ := rt.RunString(`
		var Person = pb.messageType("example.Person");
		var alice = new Person();
		alice.set("name", "Alice");
		alice.set("age", 30);
		var json = protojson.marshal(alice);
		JSON.parse(json).name + " age " + JSON.parse(json).age;
	`)
	fmt.Println("Marshal:", v.String())

	v, _ = rt.RunString(`
		var restored = protojson.unmarshal("example.Person", '{"name":"Bob","age":25}');
		restored.get("name") + " is " + restored.get("age");
	`)
	fmt.Println("Unmarshal:", v.String())
	// Output:
	// Marshal: Alice age 30
	// Unmarshal: Bob is 25
}
