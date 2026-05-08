package gojaeventloop

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/joeycumines/goja"
)

const cryptoEntropyQuota = int64(65536)

var cryptoIntegerElementBytes = map[string]int64{
	"Int8Array":         1,
	"Uint8Array":        1,
	"Uint8ClampedArray": 1,
	"Int16Array":        2,
	"Uint16Array":       2,
	"Int32Array":        4,
	"Uint32Array":       4,
	"BigInt64Array":     8,
	"BigUint64Array":    8,
}

// bindCrypto installs the retained Web Crypto subset on a fresh runtime. A
// host-provided Crypto constructor or crypto singleton is foreign state and is
// preserved without augmentation.
func (a *Adapter) bindCrypto(journal *installationJournal) (*goja.Object, error) {
	if journal == nil {
		return nil, fmt.Errorf("initialize Crypto: installation journal is unavailable")
	}
	if a.structuredCloneBrands == nil {
		return nil, fmt.Errorf("initialize Crypto: typed-array intrinsics are unavailable")
	}

	constructorValue := a.runtime.ToValue(func(goja.ConstructorCall) *goja.Object {
		panic(a.cryptoTypeError("Illegal constructor"))
	})
	constructor, ok := constructorValue.(*goja.Object)
	if !ok || constructor == nil {
		return nil, fmt.Errorf("initialize Crypto: constructor is not an object")
	}
	if err := constructor.DefineDataProperty("name", a.runtime.ToValue("Crypto"), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		return nil, fmt.Errorf("define Crypto constructor name: %w", err)
	}
	if err := constructor.DefineDataProperty("length", a.runtime.ToValue(0), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		return nil, fmt.Errorf("define Crypto constructor length: %w", err)
	}
	prototype, ok := constructor.Get("prototype").(*goja.Object)
	if !ok || prototype == nil {
		return nil, fmt.Errorf("initialize Crypto: prototype is not an object")
	}
	if err := lockWebConstructorPrototype(a.runtime, constructor, "Crypto"); err != nil {
		return nil, err
	}

	crypto := a.runtime.NewObject()
	if err := crypto.SetPrototype(prototype); err != nil {
		return nil, fmt.Errorf("set crypto prototype: %w", err)
	}
	a.setHiddenState(a.uncloneableStateStore, crypto, true)
	requireReceiver := func(value goja.Value) {
		obj, ok := value.(*goja.Object)
		if !ok || obj == nil || obj != crypto {
			panic(a.cryptoTypeError("Value of \"this\" must be of type Crypto"))
		}
	}

	getRandomValues := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		requireReceiver(call.This)
		if len(call.Arguments) == 0 {
			panic(a.cryptoTypeError("Failed to execute 'getRandomValues' on 'Crypto': 1 argument required, but only 0 present."))
		}

		arg := call.Argument(0)
		obj, ok := arg.(*goja.Object)
		if !ok || obj == nil || (!a.isDataViewObject(obj) && !a.isTypedArrayObject(obj)) {
			panic(a.cryptoTypeError("The data argument must be an ArrayBufferView"))
		}
		bytes, byteLength, ok := a.cryptoIntegerView(arg)
		if !ok {
			panic(a.throwDOMException("TypeMismatchError", "The data argument must be an integer-type TypedArray"))
		}
		if byteLength > cryptoEntropyQuota {
			panic(a.throwDOMException("QuotaExceededError", "The requested length exceeds 65,536 bytes"))
		}
		reader := a.entropyReader
		if reader == nil {
			reader = rand.Reader
		}
		if _, err := io.ReadFull(reader, bytes); err != nil {
			panic(a.runtime.NewGoError(fmt.Errorf("crypto.getRandomValues: generate random bytes: %w", err)))
		}
		return arg
	})
	if err := defineWebFunction(a.runtime, getRandomValues, "getRandomValues", 1, "define Crypto.prototype.getRandomValues"); err != nil {
		return nil, err
	}
	if err := prototype.DefineDataProperty("getRandomValues", getRandomValues, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
		return nil, fmt.Errorf("define Crypto.prototype.getRandomValues: %w", err)
	}

	randomUUID := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		requireReceiver(call.This)
		reader := a.entropyReader
		if reader == nil {
			reader = rand.Reader
		}
		uuid, err := generateUUIDv4(reader)
		if err != nil {
			panic(a.runtime.NewGoError(err))
		}
		return a.runtime.ToValue(uuid)
	})
	if err := defineWebFunction(a.runtime, randomUUID, "randomUUID", 0, "define Crypto.prototype.randomUUID"); err != nil {
		return nil, err
	}
	if err := prototype.DefineDataProperty("randomUUID", randomUUID, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
		return nil, fmt.Errorf("define Crypto.prototype.randomUUID: %w", err)
	}

	if err := defineWebTag(a.runtime, prototype, "Crypto"); err != nil {
		return nil, err
	}
	if _, err := verifyBrandedSingletonObject(a, crypto, constructor, "Crypto", []string{"randomUUID", "getRandomValues"}, nil); err != nil {
		return nil, err
	}

	// Publish only after the detached constructor, prototype, and singleton are
	// complete. Bind's ownership transaction journals both global mutations.
	if err := journal.setGlobal("Crypto", constructor); err != nil {
		return nil, err
	}
	if err := journal.setGlobal("crypto", crypto); err != nil {
		return nil, err
	}
	return crypto, nil
}

func (a *Adapter) cryptoTypeError(message string) *goja.Object {
	return a.runtime.NewTypeError(message)
}

func (a *Adapter) cryptoIntegerView(value goja.Value) ([]byte, int64, bool) {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return nil, 0, false
	}
	nameValue, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.typedArrayName, obj)
	if !ok {
		return nil, 0, false
	}
	elementBytes, ok := cryptoIntegerElementBytes[nameValue.String()]
	if !ok {
		return nil, 0, false
	}

	bufferValue, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.typedArrayBuffer, obj)
	if !ok {
		return nil, 0, false
	}
	bufferObj, ok := bufferValue.(*goja.Object)
	if !ok || bufferObj == nil {
		return nil, 0, false
	}
	buffer, ok := bufferObj.Export().(goja.ArrayBuffer)
	if !ok {
		return nil, 0, false
	}
	if buffer.Detached() {
		return []byte{}, 0, true
	}

	offsetValue, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.typedArrayByteOffset, obj)
	if !ok {
		return nil, 0, false
	}
	lengthValue, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.typedArrayLength, obj)
	if !ok {
		return nil, 0, false
	}
	offset := offsetValue.ToInteger()
	length := lengthValue.ToInteger()
	if offset < 0 || length < 0 || length > int64(^uint64(0)>>1)/elementBytes {
		return nil, 0, false
	}
	byteLength := length * elementBytes
	backing := buffer.Bytes()
	if offset > int64(len(backing)) || byteLength > int64(len(backing))-offset {
		return nil, 0, false
	}
	return backing[int(offset):int(offset+byteLength)], byteLength, true
}

// generateUUIDv4 generates a cryptographically secure UUID v4 string.
// Format: "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx" where y is 8, 9, a, or b.
func generateUUIDv4(reader io.Reader) (string, error) {
	if reader == nil {
		return "", fmt.Errorf("failed to generate random bytes: entropy reader is nil")
	}
	var uuid [16]byte
	_, err := io.ReadFull(reader, uuid[:])
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Set version (4) and variant bits per RFC 4122
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 1

	// Format as standard UUID string
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		uuid[0], uuid[1], uuid[2], uuid[3],
		uuid[4], uuid[5],
		uuid[6], uuid[7],
		uuid[8], uuid[9],
		uuid[10], uuid[11], uuid[12], uuid[13], uuid[14], uuid[15]), nil
}
