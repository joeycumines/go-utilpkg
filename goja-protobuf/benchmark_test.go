package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// BenchmarkMessageCreate benchmarks creating a new message from JS.
func BenchmarkMessageCreate(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())
	pb := rt.NewObject()
	m.setupExports(pb)
	_ = rt.Set("pb", pb)

	_, _ = rt.RunString(`const SimpleMessage = pb.messageType('test.SimpleMessage')`)

	b.ResetTimer()
	for b.Loop() {
		_, _ = rt.RunString(`new SimpleMessage()`)
	}
}

// BenchmarkFieldGetSet benchmarks getting and setting a field.
func BenchmarkFieldGetSet(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())
	pb := rt.NewObject()
	m.setupExports(pb)
	_ = rt.Set("pb", pb)

	_, _ = rt.RunString(`
		const SimpleMessage = pb.messageType('test.SimpleMessage');
		const msg = new SimpleMessage();
	`)

	b.ResetTimer()
	for b.Loop() {
		_, _ = rt.RunString(`msg.set('name', 'test'); msg.get('name');`)
	}
}

// BenchmarkEncodeDecode benchmarks binary encode/decode round-trip.
func BenchmarkEncodeDecode(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())
	pb := rt.NewObject()
	m.setupExports(pb)
	_ = rt.Set("pb", pb)

	_, _ = rt.RunString(`
		const SimpleMessage = pb.messageType('test.SimpleMessage');
		const msg = new SimpleMessage();
		msg.set('name', 'benchmark');
		msg.set('value', 42);
	`)

	b.ResetTimer()
	for b.Loop() {
		_, _ = rt.RunString(`
			const enc = pb.encode(msg);
			pb.decode(SimpleMessage, enc);
		`)
	}
}

// BenchmarkJSONRoundTrip benchmarks JSON encode/decode round-trip.
func BenchmarkJSONRoundTrip(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())
	pb := rt.NewObject()
	m.setupExports(pb)
	_ = rt.Set("pb", pb)

	_, _ = rt.RunString(`
		const SimpleMessage = pb.messageType('test.SimpleMessage');
		const msg = new SimpleMessage();
		msg.set('name', 'benchmark');
		msg.set('value', 42);
	`)

	b.ResetTimer()
	for b.Loop() {
		_, _ = rt.RunString(`
			const json = pb.toJSON(msg);
			pb.fromJSON(SimpleMessage, json);
		`)
	}
}

// BenchmarkNativeGoEncode benchmarks native Go proto.Marshal for comparison.
func BenchmarkNativeGoEncode(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())

	md, _ := m.findMessageDescriptor("test.SimpleMessage")
	msg := dynamicpb.NewMessage(md)
	msg.Set(md.Fields().ByName(protoreflect.Name("name")), protoreflect.ValueOfString("benchmark"))
	msg.Set(md.Fields().ByName(protoreflect.Name("value")), protoreflect.ValueOfInt32(42))

	_ = rt // suppress unused
	b.ResetTimer()
	for b.Loop() {
		data, _ := proto.Marshal(msg)
		newMsg := dynamicpb.NewMessage(md)
		_ = proto.Unmarshal(data, newMsg)
	}
}

// BenchmarkNativeGoFieldAccess benchmarks native Go dynamicpb field get/set.
func BenchmarkNativeGoFieldAccess(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())

	md, _ := m.findMessageDescriptor("test.SimpleMessage")
	msg := dynamicpb.NewMessage(md)
	nameField := md.Fields().ByName(protoreflect.Name("name"))

	_ = rt
	b.ResetTimer()
	for b.Loop() {
		msg.Set(nameField, protoreflect.ValueOfString("test"))
		_ = msg.Get(nameField).String()
	}
}

// BenchmarkNativeGoMessageCreate benchmarks native Go dynamicpb message creation.
func BenchmarkNativeGoMessageCreate(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())

	md, _ := m.findMessageDescriptor("test.SimpleMessage")

	_ = rt
	b.ResetTimer()
	for b.Loop() {
		_ = dynamicpb.NewMessage(md)
	}
}

// BenchmarkRepeatedField benchmarks repeated field operations.
func BenchmarkRepeatedField(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())
	pb := rt.NewObject()
	m.setupExports(pb)
	_ = rt.Set("pb", pb)

	_, _ = rt.RunString(`
		const RepeatedMessage = pb.messageType('test.RepeatedMessage');
		const msg = new RepeatedMessage();
	`)

	b.ResetTimer()
	for b.Loop() {
		_, _ = rt.RunString(`
			msg.set('items', ['a', 'b', 'c', 'd', 'e']);
			const list = msg.get('items');
			for (let i = 0; i < list.length; i++) { list.get(i); }
		`)
	}
}

// BenchmarkMapField benchmarks map field operations.
func BenchmarkMapField(b *testing.B) {
	rt := goja.New()
	m, _ := New(rt)
	m.loadDescriptorSetBytes(testDescriptorSetBytes())
	pb := rt.NewObject()
	m.setupExports(pb)
	_ = rt.Set("pb", pb)

	_, _ = rt.RunString(`
		const MapMessage = pb.messageType('test.MapMessage');
		const msg = new MapMessage();
	`)

	b.ResetTimer()
	for b.Loop() {
		_, _ = rt.RunString(`
			msg.set('tags', { k1: 'v1', k2: 'v2', k3: 'v3' });
			const m = msg.get('tags');
			m.get('k1'); m.get('k2'); m.get('k3');
		`)
	}
}
