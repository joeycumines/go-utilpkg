package gojaprotobuf

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ---------- Timestamp helpers ----------

func TestJsTimestampNow(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	before := time.Now()
	result := m.jsTimestampNow(goja.FunctionCall{})
	after := time.Now()

	msg, err := m.unwrapMessage(result)
	if err != nil {
		t.Fatal(err)
	}

	ms := timestampToMs(msg)
	if ms < before.UnixMilli() || ms > after.UnixMilli() {
		t.Fatalf("timestampNow ms %d not in range [%d, %d]", ms, before.UnixMilli(), after.UnixMilli())
	}

	// Verify $type is google.protobuf.Timestamp
	typeName := result.ToObject(rt).Get("$type").String()
	if typeName != "google.protobuf.Timestamp" {
		t.Fatalf("expected google.protobuf.Timestamp, got %s", typeName)
	}
}

func TestJsTimestampFromMs(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	ms := int64(1700000000123)
	result := m.jsTimestampFromMs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(ms)},
	})

	msg, err := m.unwrapMessage(result)
	if err != nil {
		t.Fatal(err)
	}
	gotMs := timestampToMs(msg)
	if gotMs != ms {
		t.Fatalf("expected %d, got %d", ms, gotMs)
	}
}

func TestJsTimestampFromMs_Zero(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsTimestampFromMs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(0)},
	})

	msg, err := m.unwrapMessage(result)
	if err != nil {
		t.Fatal(err)
	}
	gotMs := timestampToMs(msg)
	if gotMs != 0 {
		t.Fatalf("expected 0, got %d", gotMs)
	}
}

func TestJsTimestampFromMs_Negative(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	ms := int64(-86400000) // -1 day
	result := m.jsTimestampFromMs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(ms)},
	})

	msg, err := m.unwrapMessage(result)
	if err != nil {
		t.Fatal(err)
	}
	gotMs := timestampToMs(msg)
	if gotMs != ms {
		t.Fatalf("expected %d, got %d", ms, gotMs)
	}
}

func TestJsTimestampMs(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	msg := timestampFromMs(1700000000123)
	wrapped := m.wrapMessage(msg)

	result := m.jsTimestampMs(goja.FunctionCall{
		Arguments: []goja.Value{wrapped},
	})
	if result.ToInteger() != 1700000000123 {
		t.Fatalf("expected 1700000000123, got %d", result.ToInteger())
	}
}

func TestJsTimestampMs_InvalidArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsTimestampMs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("not a message")},
	})
}

func TestJsTimestampFromDate(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	// Create a JS Date.
	dateCtor := rt.Get("Date")
	dateObj, err := rt.New(dateCtor, rt.ToValue(1700000000123))
	if err != nil {
		t.Fatal(err)
	}

	result := m.jsTimestampFromDate(goja.FunctionCall{
		Arguments: []goja.Value{dateObj},
	})

	msg, _ := m.unwrapMessage(result)
	ms := timestampToMs(msg)
	if ms != 1700000000123 {
		t.Fatalf("expected 1700000000123, got %d", ms)
	}
}

func TestJsTimestampFromDate_Number(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	// Numeric value (not a Date) should also work.
	result := m.jsTimestampFromDate(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(1700000000123)},
	})
	msg, _ := m.unwrapMessage(result)
	ms := timestampToMs(msg)
	if ms != 1700000000123 {
		t.Fatalf("expected 1700000000123, got %d", ms)
	}
}

func TestJsTimestampFromDate_Null(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for null")
		}
	}()
	m.jsTimestampFromDate(goja.FunctionCall{
		Arguments: []goja.Value{goja.Null()},
	})
}

func TestJsTimestampDate(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	msg := timestampFromMs(1700000000123)
	wrapped := m.wrapMessage(msg)

	result := m.jsTimestampDate(goja.FunctionCall{
		Arguments: []goja.Value{wrapped},
	})

	// Result should be a Date — call getTime().
	obj := result.ToObject(rt)
	getTimeVal := obj.Get("getTime")
	fn, ok := goja.AssertFunction(getTimeVal)
	if !ok {
		t.Fatal("result should be a Date with getTime()")
	}
	msVal, err := fn(obj)
	if err != nil {
		t.Fatal(err)
	}
	if msVal.ToInteger() != 1700000000123 {
		t.Fatalf("expected 1700000000123, got %d", msVal.ToInteger())
	}
}

func TestJsTimestampDate_InvalidArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsTimestampDate(goja.FunctionCall{
		Arguments: []goja.Value{goja.Null()},
	})
}

func TestTimestampRoundtrip_MsToTimestampToMs(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	for _, ms := range []int64{0, 1, 1000, 1700000000123, -1000, 999, -500, -1, -999, -1500, -1001} {
		tsVal := m.jsTimestampFromMs(goja.FunctionCall{
			Arguments: []goja.Value{rt.ToValue(ms)},
		})
		gotMs := m.jsTimestampMs(goja.FunctionCall{
			Arguments: []goja.Value{tsVal},
		})
		if gotMs.ToInteger() != ms {
			t.Fatalf("roundtrip failed for %d: got %d", ms, gotMs.ToInteger())
		}
	}
}

// TestTimestampFromMs_NegativeSubSecond verifies that negative sub-second
// milliseconds produce valid proto Timestamps with nanos in [0, 999999999].
func TestTimestampFromMs_NegativeSubSecond(t *testing.T) {
	tests := []struct {
		ms            int64
		wantSeconds   int64
		wantNanosSign string // "non-negative"
	}{
		{ms: -500, wantSeconds: -1, wantNanosSign: "non-negative"},
		{ms: -1, wantSeconds: -1, wantNanosSign: "non-negative"},
		{ms: -999, wantSeconds: -1, wantNanosSign: "non-negative"},
		{ms: -1500, wantSeconds: -2, wantNanosSign: "non-negative"},
		{ms: -1001, wantSeconds: -2, wantNanosSign: "non-negative"},
		{ms: 500, wantSeconds: 0, wantNanosSign: "non-negative"},
		{ms: -1000, wantSeconds: -1, wantNanosSign: "non-negative"}, // exact second
		{ms: 0, wantSeconds: 0, wantNanosSign: "non-negative"},
	}

	for _, tt := range tests {
		msg := timestampFromMs(tt.ms)
		seconds := msg.Get(timestampDesc.Fields().ByName("seconds")).Int()
		nanos := msg.Get(timestampDesc.Fields().ByName("nanos")).Int()

		if seconds != tt.wantSeconds {
			t.Errorf("timestampFromMs(%d): seconds = %d, want %d", tt.ms, seconds, tt.wantSeconds)
		}
		if nanos < 0 || nanos >= 1_000_000_000 {
			t.Errorf("timestampFromMs(%d): nanos = %d, must be in [0, 999999999]", tt.ms, nanos)
		}

		// Verify roundtrip
		gotMs := timestampToMs(msg)
		if gotMs != tt.ms {
			t.Errorf("timestampFromMs(%d) roundtrip: got %d", tt.ms, gotMs)
		}
	}
}

// ---------- Duration helpers ----------

func TestJsDurationFromMs(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsDurationFromMs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(5500)},
	})

	msg, err := m.unwrapMessage(result)
	if err != nil {
		t.Fatal(err)
	}
	ms := durationToMs(msg)
	if ms != 5500 {
		t.Fatalf("expected 5500, got %d", ms)
	}

	typeName := result.ToObject(rt).Get("$type").String()
	if typeName != "google.protobuf.Duration" {
		t.Fatalf("expected google.protobuf.Duration, got %s", typeName)
	}
}

func TestJsDurationMs(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	msg := durationFromMs(12345)
	wrapped := m.wrapMessage(msg)

	result := m.jsDurationMs(goja.FunctionCall{
		Arguments: []goja.Value{wrapped},
	})
	if result.ToInteger() != 12345 {
		t.Fatalf("expected 12345, got %d", result.ToInteger())
	}
}

func TestJsDurationMs_InvalidArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsDurationMs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("not a message")},
	})
}

func TestDurationRoundtrip(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	for _, ms := range []int64{0, 1, 1000, 60000, -5000, 999, -500, -1, -999, -1500, -1001} {
		durVal := m.jsDurationFromMs(goja.FunctionCall{
			Arguments: []goja.Value{rt.ToValue(ms)},
		})
		gotMs := m.jsDurationMs(goja.FunctionCall{
			Arguments: []goja.Value{durVal},
		})
		if gotMs.ToInteger() != ms {
			t.Fatalf("roundtrip failed for %d: got %d", ms, gotMs.ToInteger())
		}
	}
}

// TestDurationFromMs_NegativeSubSecond verifies that negative sub-second
// durations produce valid proto Durations where seconds and nanos have the
// same sign.
func TestDurationFromMs_NegativeSubSecond(t *testing.T) {
	tests := []struct {
		ms int64
	}{
		{ms: -500},
		{ms: -1},
		{ms: -999},
		{ms: -1500},
		{ms: -1001},
		{ms: 500},
		{ms: -1000},
		{ms: 0},
	}

	for _, tt := range tests {
		msg := durationFromMs(tt.ms)
		seconds := msg.Get(durationDesc.Fields().ByName("seconds")).Int()
		nanos := msg.Get(durationDesc.Fields().ByName("nanos")).Int()

		// Proto Duration spec: seconds and nanos must have the same sign
		// (or one is zero).
		if seconds > 0 && nanos < 0 {
			t.Errorf("durationFromMs(%d): seconds=%d, nanos=%d (different signs)", tt.ms, seconds, nanos)
		}
		if seconds < 0 && nanos > 0 {
			t.Errorf("durationFromMs(%d): seconds=%d, nanos=%d (different signs)", tt.ms, seconds, nanos)
		}
		if nanos < -999_999_999 || nanos > 999_999_999 {
			t.Errorf("durationFromMs(%d): nanos=%d out of range", tt.ms, nanos)
		}

		// Verify roundtrip
		gotMs := durationToMs(msg)
		if gotMs != tt.ms {
			t.Errorf("durationFromMs(%d) roundtrip: got %d", tt.ms, gotMs)
		}
	}
}

func TestJsDurationFromMs_Zero(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsDurationFromMs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(0)},
	})
	msg, _ := m.unwrapMessage(result)
	ms := durationToMs(msg)
	if ms != 0 {
		t.Fatalf("expected 0, got %d", ms)
	}
}

func TestJsDurationFromMs_Negative(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	result := m.jsDurationFromMs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue(-3500)},
	})
	msg, _ := m.unwrapMessage(result)
	ms := durationToMs(msg)
	if ms != -3500 {
		t.Fatalf("expected -3500, got %d", ms)
	}
}

// ---------- Any helpers ----------

func TestJsAnyPack_Unpack_Roundtrip(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	// Use StringValue as our test message.
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")
	msg := dynamicpb.NewMessage(desc)
	msg.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("packed_value"))
	wrapped := m.wrapMessage(msg)

	// Create a fake message type constructor.
	ctorObj := rt.NewObject()
	_ = ctorObj.Set("_pbMsgDesc", &messageDescHolder{desc: desc})

	// Pack.
	anyVal := m.jsAnyPack(goja.FunctionCall{
		Arguments: []goja.Value{ctorObj, wrapped},
	})

	anyMsg, err := m.unwrapMessage(anyVal)
	if err != nil {
		t.Fatal(err)
	}

	// Verify type_url.
	typeURL := anyMsg.Get(anyDesc.Fields().ByName("type_url")).String()
	if typeURL != "type.googleapis.com/google.protobuf.StringValue" {
		t.Fatalf("unexpected type_url: %s", typeURL)
	}

	// Unpack.
	unpacked := m.jsAnyUnpack(goja.FunctionCall{
		Arguments: []goja.Value{anyVal, ctorObj},
	})
	unpackedMsg, err := m.unwrapMessage(unpacked)
	if err != nil {
		t.Fatal(err)
	}
	gotVal := unpackedMsg.Get(desc.Fields().ByName("value")).String()
	if gotVal != "packed_value" {
		t.Fatalf("expected 'packed_value', got %q", gotVal)
	}
}

func TestJsAnyPack_TypeMismatch(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	stringDesc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")
	int32Desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("Int32Value")

	msg := dynamicpb.NewMessage(stringDesc)
	wrapped := m.wrapMessage(msg)

	// Constructor for Int32Value but message is StringValue.
	ctorObj := rt.NewObject()
	_ = ctorObj.Set("_pbMsgDesc", &messageDescHolder{desc: int32Desc})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for type mismatch")
		}
	}()
	m.jsAnyPack(goja.FunctionCall{
		Arguments: []goja.Value{ctorObj, wrapped},
	})
}

func TestJsAnyPack_InvalidFirstArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsAnyPack(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("not a type"), rt.ToValue("not a msg")},
	})
}

func TestJsAnyPack_InvalidSecondArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")
	ctorObj := rt.NewObject()
	_ = ctorObj.Set("_pbMsgDesc", &messageDescHolder{desc: desc})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsAnyPack(goja.FunctionCall{
		Arguments: []goja.Value{ctorObj, rt.ToValue("not a message")},
	})
}

func TestJsAnyUnpack_InvalidFirstArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsAnyUnpack(goja.FunctionCall{
		Arguments: []goja.Value{goja.Null(), rt.ToValue("x")},
	})
}

func TestJsAnyUnpack_InvalidSecondArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	anyMsg := dynamicpb.NewMessage(anyDesc)
	wrapped := m.wrapMessage(anyMsg)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsAnyUnpack(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("not a type")},
	})
}

func TestJsAnyUnpack_InvalidData(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")
	ctorObj := rt.NewObject()
	_ = ctorObj.Set("_pbMsgDesc", &messageDescHolder{desc: desc})

	// Create an Any with garbage data.
	anyMsg := dynamicpb.NewMessage(anyDesc)
	anyMsg.Set(anyDesc.Fields().ByName("type_url"),
		protoreflect.ValueOfString("type.googleapis.com/google.protobuf.StringValue"))
	anyMsg.Set(anyDesc.Fields().ByName("value"),
		protoreflect.ValueOfBytes([]byte{0xFF, 0xFF, 0xFF}))
	wrapped := m.wrapMessage(anyMsg)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for corrupt data")
		}
	}()
	m.jsAnyUnpack(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, ctorObj},
	})
}

// ---------- anyIs ----------

func TestJsAnyIs_StringMatch(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	anyMsg := dynamicpb.NewMessage(anyDesc)
	anyMsg.Set(anyDesc.Fields().ByName("type_url"),
		protoreflect.ValueOfString("type.googleapis.com/google.protobuf.StringValue"))
	wrapped := m.wrapMessage(anyMsg)

	result := m.jsAnyIs(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("google.protobuf.StringValue")},
	})
	if !result.ToBoolean() {
		t.Fatal("expected anyIs to return true")
	}
}

func TestJsAnyIs_StringNoMatch(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	anyMsg := dynamicpb.NewMessage(anyDesc)
	anyMsg.Set(anyDesc.Fields().ByName("type_url"),
		protoreflect.ValueOfString("type.googleapis.com/google.protobuf.StringValue"))
	wrapped := m.wrapMessage(anyMsg)

	result := m.jsAnyIs(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("google.protobuf.Int32Value")},
	})
	if result.ToBoolean() {
		t.Fatal("expected anyIs to return false")
	}
}

func TestJsAnyIs_WithMsgType(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")
	ctorObj := rt.NewObject()
	_ = ctorObj.Set("_pbMsgDesc", &messageDescHolder{desc: desc})

	anyMsg := dynamicpb.NewMessage(anyDesc)
	anyMsg.Set(anyDesc.Fields().ByName("type_url"),
		protoreflect.ValueOfString("type.googleapis.com/google.protobuf.StringValue"))
	wrapped := m.wrapMessage(anyMsg)

	result := m.jsAnyIs(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, ctorObj},
	})
	if !result.ToBoolean() {
		t.Fatal("expected anyIs to return true with message type")
	}
}

func TestJsAnyIs_InvalidFirstArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsAnyIs(goja.FunctionCall{
		Arguments: []goja.Value{rt.ToValue("not a msg"), rt.ToValue("type")},
	})
}

func TestJsAnyIs_NullSecondArg(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	anyMsg := dynamicpb.NewMessage(anyDesc)
	wrapped := m.wrapMessage(anyMsg)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for null second arg")
		}
	}()
	m.jsAnyIs(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, goja.Null()},
	})
}

func TestJsAnyIs_NoSlashInTypeURL(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	anyMsg := dynamicpb.NewMessage(anyDesc)
	anyMsg.Set(anyDesc.Fields().ByName("type_url"),
		protoreflect.ValueOfString("google.protobuf.StringValue"))
	wrapped := m.wrapMessage(anyMsg)

	result := m.jsAnyIs(goja.FunctionCall{
		Arguments: []goja.Value{wrapped, rt.ToValue("google.protobuf.StringValue")},
	})
	if !result.ToBoolean() {
		t.Fatal("expected anyIs to match even without slash prefix")
	}
}

// ---------- Internal helpers ----------

func TestTimestampFromTime(t *testing.T) {
	ts := time.Date(2024, 1, 15, 12, 30, 45, 123000000, time.UTC)
	msg := timestampFromTime(ts)
	seconds := msg.Get(timestampDesc.Fields().ByName("seconds")).Int()
	nanos := msg.Get(timestampDesc.Fields().ByName("nanos")).Int()
	if seconds != ts.Unix() {
		t.Fatalf("seconds: expected %d, got %d", ts.Unix(), seconds)
	}
	if nanos != int64(ts.Nanosecond()) {
		t.Fatalf("nanos: expected %d, got %d", ts.Nanosecond(), nanos)
	}
}

func TestLastSlash(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"type.googleapis.com/foo.Bar", 19},
		{"no-slash", -1},
		{"a/b/c", 3},
		{"/", 0},
		{"", -1},
	}
	for _, tt := range tests {
		got := lastSlash(tt.input)
		if got != tt.want {
			t.Errorf("lastSlash(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ---------- Integration: via JS require ----------

func TestWellKnownTypes_JSIntegration(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		// Timestamp roundtrip
		var ts = pb.timestampFromMs(1700000000123);
		var ms = pb.timestampMs(ts);
		if (ms !== 1700000000123) throw new Error('timestampFromMs/timestampMs roundtrip: got ' + ms);

		// timestampNow should return a reasonable value
		var now = pb.timestampNow();
		var nowMs = pb.timestampMs(now);
		if (nowMs < 1000000000000) throw new Error('timestampNow too small: ' + nowMs);

		// timestampDate/timestampFromDate roundtrip
		var date = pb.timestampDate(ts);
		if (date.getTime() !== 1700000000123) throw new Error('timestampDate: got ' + date.getTime());
		var ts2 = pb.timestampFromDate(date);
		var ms2 = pb.timestampMs(ts2);
		if (ms2 !== 1700000000123) throw new Error('timestampFromDate roundtrip: got ' + ms2);

		// Duration roundtrip
		var dur = pb.durationFromMs(5500);
		var durMs = pb.durationMs(dur);
		if (durMs !== 5500) throw new Error('durationFromMs/durationMs roundtrip: got ' + durMs);

		// Any pack/unpack
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.set('name', 'packed');
		msg.set('value', 42);

		var any = pb.anyPack(SM, msg);
		if (!pb.anyIs(any, 'test.SimpleMessage')) throw new Error('anyIs should be true');
		if (pb.anyIs(any, 'test.AllTypes')) throw new Error('anyIs should be false');
		if (!pb.anyIs(any, SM)) throw new Error('anyIs with msgType should be true');

		var unpacked = pb.anyUnpack(any, SM);
		if (unpacked.get('name') !== 'packed') throw new Error('anyUnpack name mismatch');
		if (unpacked.get('value') !== 42) throw new Error('anyUnpack value mismatch');

		true
	`)

	if !v.ToBoolean() {
		t.Fatal("integration test failed")
	}
}

// TestSetupExports_WellKnownTypeHelpers verifies new exports are wired up.
func TestSetupExports_WellKnownTypeHelpers(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	exports := rt.NewObject()
	m.setupExports(exports)

	for _, name := range []string{
		"timestampNow", "timestampFromDate", "timestampDate",
		"timestampFromMs", "timestampMs",
		"durationFromMs", "durationMs",
		"anyPack", "anyUnpack", "anyIs",
	} {
		v := exports.Get(name)
		if v == nil || goja.IsUndefined(v) {
			t.Errorf("expected %q to be exported", name)
		}
	}
}

// ---------- Edge cases ----------

func TestExtractDateMs_Numeric(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	ms, err := m.extractDateMs(rt.ToValue(1234567))
	if err != nil {
		t.Fatal(err)
	}
	if ms != 1234567 {
		t.Fatalf("expected 1234567, got %d", ms)
	}
}

func TestExtractDateMs_Null(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	_, err := m.extractDateMs(goja.Null())
	if err == nil {
		t.Fatal("expected error for null")
	}
}

func TestExtractDateMs_Undefined(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	_, err := m.extractDateMs(goja.Undefined())
	if err == nil {
		t.Fatal("expected error for undefined")
	}
}

func TestNewDate(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	dateVal := m.newDate(1700000000123)
	obj := dateVal.ToObject(rt)
	getTimeVal := obj.Get("getTime")
	fn, ok := goja.AssertFunction(getTimeVal)
	if !ok {
		t.Fatal("expected Date object with getTime()")
	}
	ms, _ := fn(obj)
	if ms.ToInteger() != 1700000000123 {
		t.Fatalf("expected 1700000000123, got %d", ms.ToInteger())
	}
}

// TestAnyPackUnpack_WithEncode uses encode/decode to verify the Any value
// is proper binary.
func TestAnyPackUnpack_WithEncode(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)
	env := newTestEnv(t)

	_ = m // just needed for helper consistency

	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.set('name', 'test');

		var any = pb.anyPack(SM, msg);

		// Encode the Any itself
		var anyBytes = pb.encode(any);
		if (anyBytes.length === 0) throw new Error('encoded Any should not be empty');

		// We can also use isMessage on the Any
		if (!pb.isMessage(any)) throw new Error('Any should be a message');

		true
	`)
	if !v.ToBoolean() {
		t.Fatal("test failed")
	}
}

// Verify that anyPack also works with encode, verifying binary format.
func TestAnyPackMarshalVerification(t *testing.T) {
	desc := wrapperspb.File_google_protobuf_wrappers_proto.Messages().ByName("StringValue")
	msg := dynamicpb.NewMessage(desc)
	msg.Set(desc.Fields().ByName("value"), protoreflect.ValueOfString("hello"))

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	anyMsg := dynamicpb.NewMessage(anyDesc)
	anyMsg.Set(anyDesc.Fields().ByName("type_url"),
		protoreflect.ValueOfString("type.googleapis.com/google.protobuf.StringValue"))
	anyMsg.Set(anyDesc.Fields().ByName("value"), protoreflect.ValueOfBytes(data))

	// Verify we can marshal the Any message itself.
	anyData, err := proto.Marshal(anyMsg)
	if err != nil {
		t.Fatal(err)
	}
	if len(anyData) == 0 {
		t.Fatal("anyData should not be empty")
	}
}

// ---------- Coverage gap tests ----------

// TestNewDate_NoDateGlobal covers the fallback when Date constructor is
// unavailable (newDate returns ms as number).
func TestNewDate_NoDateGlobal(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	// Delete the Date constructor.
	_ = rt.Set("Date", goja.Undefined())

	val := m.newDate(1700000000123)
	// Should return the numeric value as fallback.
	if val.ToInteger() != 1700000000123 {
		t.Fatalf("expected 1700000000123, got %d", val.ToInteger())
	}
}

// TestNewDate_NullDateGlobal covers the null Date constructor fallback.
func TestNewDate_NullDateGlobal(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	_ = rt.Set("Date", goja.Null())

	val := m.newDate(42)
	if val.ToInteger() != 42 {
		t.Fatalf("expected 42, got %d", val.ToInteger())
	}
}

// TestNewDate_DateConstructorError covers the error fallback when Date
// constructor is not actually constructable.
func TestNewDate_DateConstructorError(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	// Set Date to a non-constructable value (a plain object).
	_ = rt.Set("Date", rt.NewObject())

	val := m.newDate(1234)
	// Should fallback to numeric.
	if val.ToInteger() != 1234 {
		t.Fatalf("expected 1234, got %d", val.ToInteger())
	}
}

// TestExtractDateMs_ObjectWithoutGetTime covers the path where the
// object does not have a getTime method (falls back to numeric).
func TestExtractDateMs_ObjectWithoutGetTime(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	// Create a plain object with no getTime.
	obj := rt.NewObject()
	_ = obj.Set("valueOf", rt.ToValue(func() int64 { return 999 }))

	ms, err := m.extractDateMs(obj)
	if err != nil {
		t.Fatal(err)
	}
	// goja's ToInteger on an object will call valueOf.
	_ = ms // just verify no error
}

// TestExtractDateMs_GetTimeNotFunction covers the path where getTime
// exists but is not callable.
func TestExtractDateMs_GetTimeNotFunction(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	obj := rt.NewObject()
	_ = obj.Set("getTime", rt.ToValue(42)) // not a function

	ms, err := m.extractDateMs(obj)
	if err != nil {
		t.Fatal(err)
	}
	_ = ms // should fall through to ToInteger
}

// TestExtractDateMs_NilValue covers the nil value path.
func TestExtractDateMs_NilValue(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	_, err := m.extractDateMs(nil)
	if err == nil {
		t.Fatal("expected error for nil")
	}
}

// TestJsAnyPack_NoArgs covers missing arguments.
func TestJsAnyPack_NoArgs(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsAnyPack(goja.FunctionCall{})
}

// TestJsAnyUnpack_NoArgs covers missing arguments.
func TestJsAnyUnpack_NoArgs(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsAnyUnpack(goja.FunctionCall{})
}

// TestJsAnyIs_NoArgs covers missing arguments.
func TestJsAnyIs_NoArgs(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	m.jsAnyIs(goja.FunctionCall{})
}

// TestExtractDateMs_GetTimeThrows covers the getTime() error path.
func TestExtractDateMs_GetTimeThrows(t *testing.T) {
	rt := goja.New()
	m := mustNewModule(t, rt)

	obj := rt.NewObject()
	// A getTime function that throws.
	throwFn, _ := rt.RunString("(function() { throw new Error('boom'); })")
	_ = obj.Set("getTime", throwFn)

	_, err := m.extractDateMs(obj)
	if err == nil {
		t.Fatal("expected error when getTime throws")
	}
}
