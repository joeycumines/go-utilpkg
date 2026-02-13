package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ---------- equals ----------

func TestJsEquals_IdenticalMessages(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	msg1 := dynamicpb.NewMessage(desc)
	msg1.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("hello"))
	msg2 := dynamicpb.NewMessage(desc)
	msg2.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("hello"))

	obj1 := m.wrapMessage(msg1)
	obj2 := m.wrapMessage(msg2)

	result := m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{obj1, obj2},
	})
	if !result.ToBoolean() {
		t.Fatal("expected equals to return true for identical messages")
	}
}

func TestJsEquals_DifferentMessages(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	msg1 := dynamicpb.NewMessage(desc)
	msg1.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("hello"))
	msg2 := dynamicpb.NewMessage(desc)
	msg2.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("world"))

	obj1 := m.wrapMessage(msg1)
	obj2 := m.wrapMessage(msg2)

	result := m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{obj1, obj2},
	})
	if result.ToBoolean() {
		t.Fatal("expected equals to return false for different messages")
	}
}

func TestJsEquals_BothEmpty(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	obj1 := m.wrapMessage(dynamicpb.NewMessage(desc))
	obj2 := m.wrapMessage(dynamicpb.NewMessage(desc))

	result := m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{obj1, obj2},
	})
	if !result.ToBoolean() {
		t.Fatal("expected equals to return true for two empty messages")
	}
}

func TestJsEquals_InvalidFirstArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid first argument")
		}
	}()
	m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("not a message"), rt.ToValue("also not")},
	})
}

func TestJsEquals_InvalidSecondArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	obj := m.wrapMessage(dynamicpb.NewMessage(desc))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid second argument")
		}
	}()
	m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{obj, rt.ToValue(42)},
	})
}

func TestJsEquals_NilFirstArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil first argument")
		}
	}()
	m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{goja.Null(), goja.Null()},
	})
}

func TestJsEquals_CrossType(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	stringDesc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")
	int32Desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("Int32Value")

	obj1 := m.wrapMessage(dynamicpb.NewMessage(stringDesc))
	obj2 := m.wrapMessage(dynamicpb.NewMessage(int32Desc))

	// proto.Equal returns false for different message types
	result := m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{obj1, obj2},
	})
	if result.ToBoolean() {
		t.Fatal("expected equals to return false for different message types")
	}
}

// ---------- clone ----------

func TestJsClone_BasicClone(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	msg := dynamicpb.NewMessage(desc)
	msg.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("original"))

	wrapped := m.wrapMessage(msg)
	cloned := m.jsClone(goja.FunctionCall{
		Arguments: []goja.Value{wrapped},
	})

	// Original and clone should be equal
	eqResult := m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, cloned},
	})
	if !eqResult.ToBoolean() {
		t.Fatal("clone should equal original")
	}

	// Modify clone, verify original unchanged
	clonedMsg, err := m.unwrapMessage(cloned)
	if err != nil {
		t.Fatal(err)
	}
	clonedMsg.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("modified"))

	origMsg, err := m.unwrapMessage(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	origVal := origMsg.Get(desc.Fields().ByName("value")).String()
	if origVal != "original" {
		t.Fatalf("original should be unchanged, got %q", origVal)
	}
}

func TestJsClone_EmptyMessage(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	wrapped := m.wrapMessage(dynamicpb.NewMessage(desc))
	cloned := m.jsClone(goja.FunctionCall{
		Arguments: []goja.Value{wrapped},
	})

	eqResult := m.jsEquals(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, cloned},
	})
	if !eqResult.ToBoolean() {
		t.Fatal("cloned empty message should equal original")
	}
}

func TestJsClone_InvalidArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid argument")
		}
	}()
	m.jsClone(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("not a message")},
	})
}

func TestJsClone_NilArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil argument")
		}
	}()
	m.jsClone(goja.FunctionCall{
		Arguments: []goja.Value{goja.Null()},
	})
}

// ---------- isMessage ----------

func TestJsIsMessage_ValidMessage(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	wrapped := m.wrapMessage(dynamicpb.NewMessage(desc))
	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{wrapped},
	})
	if !result.ToBoolean() {
		t.Fatal("expected isMessage to return true for wrapped message")
	}
}

func TestJsIsMessage_NotAMessage(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("not a message")},
	})
	if result.ToBoolean() {
		t.Fatal("expected isMessage to return false for string")
	}
}

func TestJsIsMessage_Null(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{goja.Null()},
	})
	if result.ToBoolean() {
		t.Fatal("expected isMessage to return false for null")
	}
}

func TestJsIsMessage_Undefined(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{goja.Undefined()},
	})
	if result.ToBoolean() {
		t.Fatal("expected isMessage to return false for undefined")
	}
}

func TestJsIsMessage_Number(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(42)},
	})
	if result.ToBoolean() {
		t.Fatal("expected isMessage to return false for number")
	}
}

func TestJsIsMessage_PlainObject(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	obj := rt.NewObject()
	_ = obj.Set("foo", "bar")
	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{obj},
	})
	if result.ToBoolean() {
		t.Fatal("expected isMessage to return false for plain object")
	}
}

func TestJsIsMessage_WithTypeName_Match(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	wrapped := m.wrapMessage(dynamicpb.NewMessage(desc))
	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("google.protobuf.StringValue")},
	})
	if !result.ToBoolean() {
		t.Fatal("expected isMessage to return true for matching type name")
	}
}

func TestJsIsMessage_WithTypeName_NoMatch(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	wrapped := m.wrapMessage(dynamicpb.NewMessage(desc))
	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("some.OtherType")},
	})
	if result.ToBoolean() {
		t.Fatal("expected isMessage to return false for non-matching type name")
	}
}

func TestJsIsMessage_NotMessageWithTypeName(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("string"), rt.ToValue("some.Type")},
	})
	if result.ToBoolean() {
		t.Fatal("expected isMessage to return false for non-message even with type name")
	}
}

func TestJsIsMessage_WithUndefinedTypeName(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	wrapped := m.wrapMessage(dynamicpb.NewMessage(desc))
	// Second arg is undefined — should return true (no type check).
	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, goja.Undefined()},
	})
	if !result.ToBoolean() {
		t.Fatal("expected isMessage to return true when typeName is undefined")
	}
}

func TestJsIsMessage_WithNullTypeName(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	wrapped := m.wrapMessage(dynamicpb.NewMessage(desc))
	// Second arg is null — should return true (no type check).
	result := m.jsIsMessage(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, goja.Null()},
	})
	if !result.ToBoolean() {
		t.Fatal("expected isMessage to return true when typeName is null")
	}
}

// ---------- Integration: via setupExports ----------

func TestSetupExports_EqualsCloneIsMessage(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	exports := rt.NewObject()
	m.setupExports(exports)

	// Verify new functions are wired up.
	for _, name := range []string{"equals", "clone", "isMessage", "isFieldSet", "clearField"} {
		v := exports.Get(name)
		if v == nil || goja.IsUndefined(v) {
			t.Errorf("expected %q to be exported", name)
		}
	}
}

// ---------- isFieldSet ----------

func TestJsIsFieldSet_FieldSet(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	msg := dynamicpb.NewMessage(desc)
	msg.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("hello"))

	wrapped := m.wrapMessage(msg)
	result := m.jsIsFieldSet(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("value")},
	})
	if !result.ToBoolean() {
		t.Fatal("expected isFieldSet to return true for set field")
	}
}

func TestJsIsFieldSet_FieldNotSet(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	msg := dynamicpb.NewMessage(desc)
	wrapped := m.wrapMessage(msg)
	result := m.jsIsFieldSet(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("value")},
	})
	if result.ToBoolean() {
		t.Fatal("expected isFieldSet to return false for unset field")
	}
}

func TestJsIsFieldSet_InvalidMessage(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid message")
		}
	}()
	m.jsIsFieldSet(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("not a message"), rt.ToValue("field")},
	})
}

func TestJsIsFieldSet_InvalidFieldName(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	wrapped := m.wrapMessage(dynamicpb.NewMessage(desc))
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid field name")
		}
	}()
	m.jsIsFieldSet(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("nonexistent")},
	})
}

// ---------- clearField ----------

func TestJsClearField_ClearsSetField(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	msg := dynamicpb.NewMessage(desc)
	msg.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("hello"))

	wrapped := m.wrapMessage(msg)

	// Verify field is set
	if !msg.Has(desc.Fields().ByName("value")) {
		t.Fatal("precondition: field should be set")
	}

	result := m.jsClearField(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("value")},
	})
	if !goja.IsUndefined(result) {
		t.Fatal("clearField should return undefined")
	}

	// Verify field is cleared
	if msg.Has(desc.Fields().ByName("value")) {
		t.Fatal("field should be cleared after clearField")
	}
}

func TestJsClearField_InvalidMessage(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid message")
		}
	}()
	m.jsClearField(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(42), rt.ToValue("field")},
	})
}

func TestJsClearField_InvalidFieldName(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")

	wrapped := m.wrapMessage(dynamicpb.NewMessage(desc))
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid field name")
		}
	}()
	m.jsClearField(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("nonexistent")},
	})
}

// mustNewModule is a test helper that creates a Module or fails.
func mustNewModule(t *testing.T, rt *goja.Runtime) *Module {
	t.Helper()
	m, err := New(rt)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
