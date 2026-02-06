//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-063: Date API Verification Tests
// Tests verify Goja's native support for:
// - Date constructor (no args, timestamp, string, year/month/day/hour/min/sec/ms)
// - Date.now(), Date.parse(), Date.UTC()
// - Getter methods (getTime, getFullYear, getMonth, getDate, getDay, getHours, etc.)
// - UTC getter methods (getUTCFullYear, getUTCMonth, etc.)
// - Setter methods (setTime, setFullYear, setMonth, etc.)
// - String conversion methods (toISOString, toJSON, toDateString, toTimeString, toString)
// - Locale methods (toLocaleDateString, toLocaleTimeString, toLocaleString)
// - getTimezoneOffset()
// - valueOf()
// - Invalid Date handling (NaN)
//
// STATUS: Date is NATIVE to Goja
// ===============================================

func newDateTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("Bind failed: %v", err)
	}

	cleanup := func() {
		loop.Shutdown(context.Background())
	}

	return adapter, runtime, cleanup
}

// ===============================================
// Date Constructor Tests
// ===============================================

func TestDate_Constructor_NoArgs(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var before = Date.now();
		var d = new Date();
		var after = Date.now();
		var time = d.getTime();
		time >= before && time <= after;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date() with no args should return current time")
	}
}

func TestDate_Constructor_Timestamp(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	// Timestamp for 2020-01-01T00:00:00.000Z
	script := `
		var d = new Date(1577836800000);
		d.getTime() === 1577836800000;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date(timestamp) should create date from milliseconds")
	}
}

func TestDate_Constructor_String(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date('2020-06-15T10:30:00.000Z');
		d.getUTCFullYear() === 2020 &&
		d.getUTCMonth() === 5 && // June (0-indexed)
		d.getUTCDate() === 15 &&
		d.getUTCHours() === 10 &&
		d.getUTCMinutes() === 30;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date(string) should parse ISO date string")
	}
}

func TestDate_Constructor_Components(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	// Using UTC hours to avoid timezone issues
	script := `
		var d = new Date(2021, 11, 25, 14, 30, 45, 123); // Dec 25, 2021 14:30:45.123 local time
		d.getFullYear() === 2021 &&
		d.getMonth() === 11 && // December (0-indexed)
		d.getDate() === 25 &&
		d.getHours() === 14 &&
		d.getMinutes() === 30 &&
		d.getSeconds() === 45 &&
		d.getMilliseconds() === 123;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date(year, month, day, hour, min, sec, ms) should set all components")
	}
}

func TestDate_Constructor_YearMonthOnly(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2022, 5); // June 2022
		d.getFullYear() === 2022 &&
		d.getMonth() === 5 &&
		d.getDate() === 1; // defaults to 1
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date(year, month) should default date to 1")
	}
}

// ===============================================
// Static Methods Tests
// ===============================================

func TestDate_Now(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var t1 = Date.now();
		var t2 = Date.now();
		typeof t1 === 'number' && typeof t2 === 'number' && t2 >= t1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date.now() should return current timestamp as number")
	}
}

func TestDate_Parse(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var timestamp = Date.parse('2020-01-01T00:00:00.000Z');
		timestamp === 1577836800000;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date.parse() should return timestamp from ISO string")
	}
}

func TestDate_UTC(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var timestamp = Date.UTC(2020, 0, 1, 0, 0, 0, 0); // Jan 1, 2020 00:00:00.000 UTC
		timestamp === 1577836800000;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date.UTC() should return timestamp from UTC components")
	}
}

// ===============================================
// Getter Methods Tests
// ===============================================

func TestDate_GetTime(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(1577836800000);
		d.getTime() === 1577836800000;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("getTime() should return timestamp")
	}
}

func TestDate_GetFullYear(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2023, 6, 15);
		d.getFullYear() === 2023;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("getFullYear() should return 4-digit year")
	}
}

func TestDate_GetMonth(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2023, 6, 15); // July
		d.getMonth() === 6; // 0-indexed
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("getMonth() should return 0-indexed month")
	}
}

func TestDate_GetDate(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2023, 6, 15);
		d.getDate() === 15;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("getDate() should return day of month")
	}
}

func TestDate_GetDay(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	// Jan 1, 2020 was a Wednesday (day 3)
	script := `
		var d = new Date('2020-01-01T12:00:00.000Z');
		d.getUTCDay() === 3;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("getDay() should return day of week (0=Sunday)")
	}
}

func TestDate_GetHoursMinutesSecondsMilliseconds(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2023, 5, 15, 14, 30, 45, 123);
		d.getHours() === 14 &&
		d.getMinutes() === 30 &&
		d.getSeconds() === 45 &&
		d.getMilliseconds() === 123;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("getHours/Minutes/Seconds/Milliseconds should return time components")
	}
}

// ===============================================
// UTC Getter Methods Tests
// ===============================================

func TestDate_UTCGetters(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date('2023-07-15T14:30:45.123Z');
		d.getUTCFullYear() === 2023 &&
		d.getUTCMonth() === 6 && // July
		d.getUTCDate() === 15 &&
		d.getUTCHours() === 14 &&
		d.getUTCMinutes() === 30 &&
		d.getUTCSeconds() === 45 &&
		d.getUTCMilliseconds() === 123;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("UTC getter methods should return UTC time components")
	}
}

// ===============================================
// Setter Methods Tests
// ===============================================

func TestDate_SetTime(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date();
		d.setTime(1577836800000);
		d.getTime() === 1577836800000;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("setTime() should set timestamp")
	}
}

func TestDate_SetFullYear(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15);
		d.setFullYear(2025);
		d.getFullYear() === 2025;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("setFullYear() should change year")
	}
}

func TestDate_SetMonth(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15);
		d.setMonth(11); // December
		d.getMonth() === 11;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("setMonth() should change month")
	}
}

func TestDate_SetDate(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15);
		d.setDate(25);
		d.getDate() === 25;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("setDate() should change day of month")
	}
}

func TestDate_SetHoursMinutesSecondsMilliseconds(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15, 0, 0, 0, 0);
		d.setHours(14);
		d.setMinutes(30);
		d.setSeconds(45);
		d.setMilliseconds(123);
		d.getHours() === 14 &&
		d.getMinutes() === 30 &&
		d.getSeconds() === 45 &&
		d.getMilliseconds() === 123;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("setHours/Minutes/Seconds/Milliseconds should set time components")
	}
}

// ===============================================
// String Conversion Tests
// ===============================================

func TestDate_ToISOString(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date('2020-06-15T10:30:00.000Z');
		d.toISOString() === '2020-06-15T10:30:00.000Z';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toISOString() should return ISO 8601 format")
	}
}

func TestDate_ToJSON(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date('2020-06-15T10:30:00.000Z');
		d.toJSON() === '2020-06-15T10:30:00.000Z';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toJSON() should return ISO 8601 format (same as toISOString)")
	}
}

func TestDate_ToString(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15);
		var str = d.toString();
		typeof str === 'string' && str.includes('2020');
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toString() should return string representation with year")
	}
}

func TestDate_ToDateString(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15);
		var str = d.toDateString();
		typeof str === 'string' && str.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toDateString() should return date-only string")
	}
}

func TestDate_ToTimeString(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15, 14, 30, 45);
		var str = d.toTimeString();
		typeof str === 'string' && str.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toTimeString() should return time-only string")
	}
}

// ===============================================
// Locale Methods Tests
// ===============================================

func TestDate_ToLocaleString(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15, 14, 30);
		var str = d.toLocaleString();
		typeof str === 'string' && str.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toLocaleString() should return locale-formatted string")
	}
}

func TestDate_ToLocaleDateString(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15);
		var str = d.toLocaleDateString();
		typeof str === 'string' && str.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toLocaleDateString() should return locale-formatted date string")
	}
}

func TestDate_ToLocaleTimeString(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(2020, 5, 15, 14, 30, 45);
		var str = d.toLocaleTimeString();
		typeof str === 'string' && str.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toLocaleTimeString() should return locale-formatted time string")
	}
}

// ===============================================
// Other Methods Tests
// ===============================================

func TestDate_GetTimezoneOffset(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date();
		var offset = d.getTimezoneOffset();
		typeof offset === 'number' && offset >= -840 && offset <= 720; // -14h to +12h in minutes
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("getTimezoneOffset() should return offset in minutes")
	}
}

func TestDate_ValueOf(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date(1577836800000);
		d.valueOf() === 1577836800000 && d.valueOf() === d.getTime();
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("valueOf() should return timestamp (same as getTime)")
	}
}

// ===============================================
// Invalid Date Tests
// ===============================================

func TestDate_InvalidDate_NaN(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date('invalid');
		isNaN(d.getTime()) && d.toString() === 'Invalid Date';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Invalid date should have NaN time and 'Invalid Date' string")
	}
}

func TestDate_InvalidDate_ToISOString_Throws(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `
		var d = new Date('invalid');
		var threw = false;
		try {
			d.toISOString();
		} catch (e) {
			threw = true;
		}
		threw;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("toISOString() on invalid date should throw RangeError")
	}
}

// ===============================================
// Type Verification
// ===============================================

func TestDate_TypeExists(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	script := `typeof Date === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Date constructor should exist (NATIVE)")
	}
	t.Log("Date: NATIVE")
}

func TestDate_MethodsExist(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	methods := []string{
		"getTime", "getFullYear", "getMonth", "getDate", "getDay",
		"getHours", "getMinutes", "getSeconds", "getMilliseconds",
		"getUTCFullYear", "getUTCMonth", "getUTCDate", "getUTCDay",
		"getUTCHours", "getUTCMinutes", "getUTCSeconds", "getUTCMilliseconds",
		"setTime", "setFullYear", "setMonth", "setDate",
		"setHours", "setMinutes", "setSeconds", "setMilliseconds",
		"toISOString", "toJSON", "toString", "toDateString", "toTimeString",
		"toLocaleString", "toLocaleDateString", "toLocaleTimeString",
		"getTimezoneOffset", "valueOf",
	}
	for _, method := range methods {
		t.Run("Date.prototype."+method, func(t *testing.T) {
			script := `typeof (new Date()).` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Date.prototype.%s should be a function (NATIVE)", method)
			}
		})
	}
}

func TestDate_StaticMethodsExist(t *testing.T) {
	_, runtime, cleanup := newDateTestAdapter(t)
	defer cleanup()

	staticMethods := []string{"now", "parse", "UTC"}
	for _, method := range staticMethods {
		t.Run("Date."+method, func(t *testing.T) {
			script := `typeof Date.` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Date.%s should be a function (NATIVE)", method)
			}
		})
	}
}
