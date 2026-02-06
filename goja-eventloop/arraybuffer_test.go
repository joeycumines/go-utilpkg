//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-061: ArrayBuffer/DataView/TypedArray tests
// Tests verify Goja's native support for:
// - ArrayBuffer: constructor, byteLength, slice(), isView()
// - DataView: constructor, getInt8/setInt8, getUint16/setUint16, getFloat32/setFloat32 (littleEndian)
// - TypedArray: Uint8Array, Int32Array, Float64Array - from ArrayBuffer, from(iterable), slice(), subarray()
// - SharedArrayBuffer (may not exist, test gracefully)
// - Atomics (may not exist, test gracefully)
//
// STATUS: ArrayBuffer, DataView, TypedArrays are NATIVE to Goja
// ===============================================

func newArrayBufferTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
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
// ArrayBuffer Tests
// ===============================================

func TestArrayBuffer_Constructor(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(16);
		buf instanceof ArrayBuffer;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("ArrayBuffer constructor failed")
	}
}

func TestArrayBuffer_ByteLength(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf8 = new ArrayBuffer(8);
		var buf32 = new ArrayBuffer(32);
		var buf0 = new ArrayBuffer(0);
		buf8.byteLength === 8 && buf32.byteLength === 32 && buf0.byteLength === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("ArrayBuffer byteLength failed")
	}
}

func TestArrayBuffer_Slice(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(16);
		var view = new Uint8Array(buf);
		for (var i = 0; i < 16; i++) view[i] = i;
		
		var slice = buf.slice(4, 8);
		var sliceView = new Uint8Array(slice);
		slice.byteLength === 4 && sliceView[0] === 4 && sliceView[3] === 7;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("ArrayBuffer slice failed")
	}
}

func TestArrayBuffer_SliceNegativeIndices(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(10);
		var view = new Uint8Array(buf);
		for (var i = 0; i < 10; i++) view[i] = i;
		
		var slice = buf.slice(-3); // Last 3 bytes
		var sliceView = new Uint8Array(slice);
		slice.byteLength === 3 && sliceView[0] === 7 && sliceView[2] === 9;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("ArrayBuffer slice with negative indices failed")
	}
}

func TestArrayBuffer_IsView(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(16);
		var u8 = new Uint8Array(buf);
		var dv = new DataView(buf);
		
		ArrayBuffer.isView(u8) === true && 
		ArrayBuffer.isView(dv) === true && 
		ArrayBuffer.isView(buf) === false &&
		ArrayBuffer.isView({}) === false &&
		ArrayBuffer.isView(null) === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("ArrayBuffer.isView failed")
	}
}

// ===============================================
// DataView Tests
// ===============================================

func TestDataView_Constructor(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(16);
		var dv = new DataView(buf);
		dv instanceof DataView && dv.buffer === buf && dv.byteLength === 16 && dv.byteOffset === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView constructor failed")
	}
}

func TestDataView_ConstructorWithOffset(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(16);
		var dv = new DataView(buf, 4, 8);
		dv.byteOffset === 4 && dv.byteLength === 8;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView constructor with offset/length failed")
	}
}

func TestDataView_Int8(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(4);
		var dv = new DataView(buf);
		dv.setInt8(0, 127);
		dv.setInt8(1, -128);
		dv.setInt8(2, 0);
		dv.setInt8(3, -1);
		dv.getInt8(0) === 127 && dv.getInt8(1) === -128 && dv.getInt8(2) === 0 && dv.getInt8(3) === -1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView getInt8/setInt8 failed")
	}
}

func TestDataView_Uint8(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(4);
		var dv = new DataView(buf);
		dv.setUint8(0, 0);
		dv.setUint8(1, 127);
		dv.setUint8(2, 255);
		dv.setUint8(3, 128);
		dv.getUint8(0) === 0 && dv.getUint8(1) === 127 && dv.getUint8(2) === 255 && dv.getUint8(3) === 128;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView getUint8/setUint8 failed")
	}
}

func TestDataView_Uint16_LittleEndian(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(4);
		var dv = new DataView(buf);
		dv.setUint16(0, 0x1234, true); // little-endian
		dv.setUint16(2, 0xABCD, true);
		dv.getUint16(0, true) === 0x1234 && dv.getUint16(2, true) === 0xABCD;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView Uint16 little-endian failed")
	}
}

func TestDataView_Uint16_BigEndian(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(4);
		var dv = new DataView(buf);
		dv.setUint16(0, 0x1234, false); // big-endian (default)
		dv.setUint16(2, 0xABCD);        // big-endian (default)
		dv.getUint16(0) === 0x1234 && dv.getUint16(2, false) === 0xABCD;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView Uint16 big-endian failed")
	}
}

func TestDataView_Float32_LittleEndian(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(8);
		var dv = new DataView(buf);
		dv.setFloat32(0, 3.14, true); // little-endian
		dv.setFloat32(4, -1.5, true);
		Math.abs(dv.getFloat32(0, true) - 3.14) < 0.0001 && 
		Math.abs(dv.getFloat32(4, true) - (-1.5)) < 0.0001;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView Float32 little-endian failed")
	}
}

func TestDataView_Float64(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(16);
		var dv = new DataView(buf);
		dv.setFloat64(0, Math.PI, true);
		dv.setFloat64(8, -Math.E, true);
		Math.abs(dv.getFloat64(0, true) - Math.PI) < 0.0000001 && 
		Math.abs(dv.getFloat64(8, true) - (-Math.E)) < 0.0000001;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView Float64 failed")
	}
}

// ===============================================
// TypedArray Tests - Uint8Array
// ===============================================

func TestUint8Array_FromArrayBuffer(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(4);
		var u8 = new Uint8Array(buf);
		u8[0] = 1; u8[1] = 2; u8[2] = 3; u8[3] = 4;
		u8.length === 4 && u8.buffer === buf && u8.byteLength === 4 && u8.byteOffset === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Uint8Array from ArrayBuffer failed")
	}
}

func TestUint8Array_From_Iterable(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var u8 = Uint8Array.from([10, 20, 30, 40]);
		u8.length === 4 && u8[0] === 10 && u8[1] === 20 && u8[2] === 30 && u8[3] === 40;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Uint8Array.from(iterable) failed")
	}
}

func TestUint8Array_Slice(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var u8 = Uint8Array.from([0, 1, 2, 3, 4, 5]);
		var sliced = u8.slice(2, 5);
		sliced.length === 3 && sliced[0] === 2 && sliced[1] === 3 && sliced[2] === 4 && sliced.buffer !== u8.buffer;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Uint8Array.slice() failed")
	}
}

func TestUint8Array_Subarray(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var u8 = Uint8Array.from([0, 1, 2, 3, 4, 5]);
		var sub = u8.subarray(2, 5);
		sub.length === 3 && sub[0] === 2 && sub[1] === 3 && sub[2] === 4 && sub.buffer === u8.buffer;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Uint8Array.subarray() failed (shares buffer)")
	}
}

// ===============================================
// TypedArray Tests - Int32Array
// ===============================================

func TestInt32Array_FromArrayBuffer(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(16); // 4 x 4-byte integers
		var i32 = new Int32Array(buf);
		i32[0] = 100; i32[1] = -200; i32[2] = 300; i32[3] = -400;
		i32.length === 4 && i32[0] === 100 && i32[1] === -200 && i32[3] === -400;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Int32Array from ArrayBuffer failed")
	}
}

func TestInt32Array_From_Iterable(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var i32 = Int32Array.from([2147483647, -2147483648, 0]);
		i32.length === 3 && i32[0] === 2147483647 && i32[1] === -2147483648 && i32[2] === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Int32Array.from(iterable) failed")
	}
}

func TestInt32Array_SliceAndSubarray(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var i32 = Int32Array.from([1, 2, 3, 4, 5]);
		var sliced = i32.slice(1, 4);
		var sub = i32.subarray(1, 4);
		sliced.length === 3 && sub.length === 3 && 
		sliced.buffer !== i32.buffer && sub.buffer === i32.buffer;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Int32Array slice vs subarray buffer sharing failed")
	}
}

// ===============================================
// TypedArray Tests - Float64Array
// ===============================================

func TestFloat64Array_FromArrayBuffer(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var buf = new ArrayBuffer(24); // 3 x 8-byte floats
		var f64 = new Float64Array(buf);
		f64[0] = Math.PI; f64[1] = Math.E; f64[2] = -Infinity;
		f64.length === 3 && Math.abs(f64[0] - Math.PI) < 0.0000001 && f64[2] === -Infinity;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Float64Array from ArrayBuffer failed")
	}
}

func TestFloat64Array_From_Iterable(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var f64 = Float64Array.from([1.1, 2.2, 3.3]);
		f64.length === 3 && Math.abs(f64[0] - 1.1) < 0.0001 && Math.abs(f64[2] - 3.3) < 0.0001;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Float64Array.from(iterable) failed")
	}
}

func TestFloat64Array_SpecialValues(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `
		var f64 = Float64Array.from([Infinity, -Infinity, NaN, 0, -0]);
		f64[0] === Infinity && f64[1] === -Infinity && isNaN(f64[2]) && f64[3] === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Float64Array special values failed")
	}
}

// ===============================================
// SharedArrayBuffer Tests (graceful)
// ===============================================

func TestSharedArrayBuffer_Exists(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `typeof SharedArrayBuffer`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	result := v.String()
	if result == "function" {
		t.Log("SharedArrayBuffer: NATIVE (available)")
	} else if result == "undefined" {
		t.Log("SharedArrayBuffer: NOT AVAILABLE (graceful skip)")
		t.Skip("SharedArrayBuffer not supported in this Goja version")
	} else {
		t.Errorf("Unexpected SharedArrayBuffer type: %s", result)
	}
}

func TestSharedArrayBuffer_Constructor(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	// Check if SharedArrayBuffer exists first
	check, _ := runtime.RunString(`typeof SharedArrayBuffer`)
	if check.String() == "undefined" {
		t.Skip("SharedArrayBuffer not supported in this Goja version")
	}

	script := `
		var sab = new SharedArrayBuffer(16);
		sab.byteLength === 16;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("SharedArrayBuffer constructor failed")
	}
}

// ===============================================
// Atomics Tests (graceful)
// ===============================================

func TestAtomics_Exists(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `typeof Atomics`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	result := v.String()
	if result == "object" {
		t.Log("Atomics: NATIVE (available)")
	} else if result == "undefined" {
		t.Log("Atomics: NOT AVAILABLE (graceful skip)")
		t.Skip("Atomics not supported in this Goja version")
	} else {
		t.Errorf("Unexpected Atomics type: %s", result)
	}
}

func TestAtomics_Add(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	// Check if Atomics and SharedArrayBuffer exist
	check, _ := runtime.RunString(`typeof Atomics === 'object' && typeof SharedArrayBuffer === 'function'`)
	if !check.ToBoolean() {
		t.Skip("Atomics or SharedArrayBuffer not supported in this Goja version")
	}

	script := `
		var sab = new SharedArrayBuffer(4);
		var i32 = new Int32Array(sab);
		i32[0] = 10;
		var oldValue = Atomics.add(i32, 0, 5);
		oldValue === 10 && i32[0] === 15;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Atomics.add failed")
	}
}

func TestAtomics_CompareExchange(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	check, _ := runtime.RunString(`typeof Atomics === 'object' && typeof SharedArrayBuffer === 'function'`)
	if !check.ToBoolean() {
		t.Skip("Atomics or SharedArrayBuffer not supported in this Goja version")
	}

	script := `
		var sab = new SharedArrayBuffer(4);
		var i32 = new Int32Array(sab);
		i32[0] = 5;
		var result1 = Atomics.compareExchange(i32, 0, 5, 10);
		var result2 = Atomics.compareExchange(i32, 0, 5, 20); // Won't match
		result1 === 5 && i32[0] === 10 && result2 === 10;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Atomics.compareExchange failed")
	}
}

// ===============================================
// Type Verification
// ===============================================

func TestArrayBuffer_TypeExists(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `typeof ArrayBuffer === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("ArrayBuffer constructor should exist (NATIVE)")
	}
	t.Log("ArrayBuffer: NATIVE")
}

func TestDataView_TypeExists(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	script := `typeof DataView === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DataView constructor should exist (NATIVE)")
	}
	t.Log("DataView: NATIVE")
}

func TestTypedArrays_TypesExist(t *testing.T) {
	_, runtime, cleanup := newArrayBufferTestAdapter(t)
	defer cleanup()

	types := []string{
		"Uint8Array", "Int8Array", "Uint8ClampedArray",
		"Uint16Array", "Int16Array",
		"Uint32Array", "Int32Array",
		"Float32Array", "Float64Array",
	}
	for _, typeName := range types {
		t.Run(typeName, func(t *testing.T) {
			script := `typeof ` + typeName + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("%s constructor should exist (NATIVE)", typeName)
			} else {
				t.Logf("%s: NATIVE", typeName)
			}
		})
	}
}
