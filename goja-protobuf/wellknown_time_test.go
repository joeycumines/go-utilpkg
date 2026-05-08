package gojaprotobuf

import (
	"testing"
	"time"

	"github.com/joeycumines/goja"
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
