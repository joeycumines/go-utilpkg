package gojaeventloop

import (
	"fmt"

	"github.com/joeycumines/goja"
)

// DOMException class

// DOMException error codes (from the DOM spec)
const (
	DOMExceptionIndexSizeErr             = 1
	DOMExceptionDOMStringSizeErr         = 2 // Deprecated, historical
	DOMExceptionHierarchyRequestErr      = 3
	DOMExceptionWrongDocumentErr         = 4
	DOMExceptionInvalidCharacterErr      = 5
	DOMExceptionNoDataAllowedErr         = 6 // Deprecated, historical
	DOMExceptionNoModificationAllowedErr = 7
	DOMExceptionNotFoundErr              = 8
	DOMExceptionNotSupportedErr          = 9
	DOMExceptionInUseAttributeErr        = 10
	DOMExceptionInvalidStateErr          = 11
	DOMExceptionSyntaxErr                = 12
	DOMExceptionInvalidModificationErr   = 13
	DOMExceptionNamespaceErr             = 14
	DOMExceptionInvalidAccessErr         = 15
	DOMExceptionValidationErr            = 16 // Deprecated, historical
	DOMExceptionTypeMismatchErr          = 17
	DOMExceptionSecurityErr              = 18
	DOMExceptionNetworkErr               = 19
	DOMExceptionAbortErr                 = 20
	DOMExceptionURLMismatchErr           = 21
	DOMExceptionQuotaExceededErr         = 22
	DOMExceptionTimeoutErr               = 23
	DOMExceptionInvalidNodeTypeErr       = 24
	DOMExceptionDataCloneErr             = 25
)

// domExceptionNameToCode maps error names to legacy codes.
var domExceptionNameToCode = map[string]int{
	"IndexSizeError":             DOMExceptionIndexSizeErr,
	"HierarchyRequestError":      DOMExceptionHierarchyRequestErr,
	"WrongDocumentError":         DOMExceptionWrongDocumentErr,
	"InvalidCharacterError":      DOMExceptionInvalidCharacterErr,
	"NoModificationAllowedError": DOMExceptionNoModificationAllowedErr,
	"NotFoundError":              DOMExceptionNotFoundErr,
	"NotSupportedError":          DOMExceptionNotSupportedErr,
	"InUseAttributeError":        DOMExceptionInUseAttributeErr,
	"InvalidStateError":          DOMExceptionInvalidStateErr,
	"SyntaxError":                DOMExceptionSyntaxErr,
	"InvalidModificationError":   DOMExceptionInvalidModificationErr,
	"NamespaceError":             DOMExceptionNamespaceErr,
	"InvalidAccessError":         DOMExceptionInvalidAccessErr,
	"TypeMismatchError":          DOMExceptionTypeMismatchErr,
	"SecurityError":              DOMExceptionSecurityErr,
	"NetworkError":               DOMExceptionNetworkErr,
	"AbortError":                 DOMExceptionAbortErr,
	"URLMismatchError":           DOMExceptionURLMismatchErr,
	"QuotaExceededError":         DOMExceptionQuotaExceededErr,
	"TimeoutError":               DOMExceptionTimeoutErr,
	"InvalidNodeTypeError":       DOMExceptionInvalidNodeTypeErr,
	"DataCloneError":             DOMExceptionDataCloneErr,
	// New error names (code 0)
	"EncodingError":    0,
	"NotReadableError": 0,
	"UnknownError":     0,
	"ConstraintError":  0,
	"DataError":        0,
	"TransactionError": 0, // Deprecated
	"ReadOnlyError":    0,
	"VersionError":     0,
	"OperationError":   0,
	"NotAllowedError":  0,
	"OptOutError":      0, // Deprecated
}

// domExceptionWrapper wraps DOMException data.
type domExceptionWrapper struct {
	prototype *goja.Object
	message   string
	name      string
	code      int
}

func (a *Adapter) domExceptionState(obj *goja.Object) (*domExceptionWrapper, bool) {
	if obj == nil {
		return nil, false
	}
	state := a.hiddenState(a.domExceptionStateStore, obj)
	if state == nil || goja.IsUndefined(state) || goja.IsNull(state) {
		return nil, false
	}
	wrapper, ok := state.Export().(*domExceptionWrapper)
	if !ok || wrapper == nil {
		return nil, false
	}
	return wrapper, true
}

func (a *Adapter) domExceptionValue(value goja.Value) *domExceptionWrapper {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		panic(a.runtime.NewTypeError("DOMException getter called on incompatible receiver"))
	}
	wrapper, ok := a.domExceptionState(obj)
	if !ok {
		panic(a.runtime.NewTypeError("DOMException getter called on incompatible receiver"))
	}
	return wrapper
}

func (a *Adapter) newDOMExceptionObject(message, name string, objectPrototype, standardPrototype *goja.Object) *goja.Object {
	errorConstructor, ok := a.structuredCloneConstructor("Error")
	if !ok {
		panic(a.runtime.NewTypeError("DOMException cannot initialize Error data"))
	}
	obj, err := errorConstructor(nil)
	if err != nil {
		a.panicJSException(err)
	}
	if objectPrototype == nil {
		objectPrototype = standardPrototype
	}
	if objectPrototype == nil {
		panic(a.runtime.NewTypeError("DOMException prototype is unavailable"))
	}
	if err := obj.SetPrototype(objectPrototype); err != nil {
		a.panicJSException(wrapRuntimeError("set DOMException prototype", err))
	}
	code := 0
	if legacyCode, exists := domExceptionNameToCode[name]; exists {
		code = legacyCode
	}
	a.setHiddenState(a.domExceptionStateStore, obj, &domExceptionWrapper{
		message:   message,
		name:      name,
		code:      code,
		prototype: standardPrototype,
	})
	return obj
}

// throwDOMException creates a DOMException object and returns it as a goja.Value
// suitable for use with panic() to throw it as a JS exception.
func (a *Adapter) throwDOMException(name, message string) goja.Value {
	if a.domExceptionPrototype != nil {
		return a.newDOMExceptionObject(message, name, a.domExceptionPrototype, a.domExceptionPrototype)
	}
	return a.runtime.NewTypeError(name + ": " + message)
}

// domExceptionConstructor creates the DOMException constructor for JavaScript.
// Usage: new DOMException(message?, name?)
// message defaults to empty string, name defaults to "Error"
func (a *Adapter) domExceptionConstructor(call goja.ConstructorCall) *goja.Object {
	message := ""
	name := "Error"

	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
		message = a.webIDLString(call.Argument(0))
	}
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		name = a.webIDLString(call.Argument(1))
	}
	standardPrototype := a.domExceptionPrototype
	return a.newDOMExceptionObject(message, name, call.This.Prototype(), standardPrototype)
}

// bindDOMExceptionConstants adds all constant properties to DOMException.
func (a *Adapter) bindDOMExceptionConstants(domExceptionObj, errorPrototype *goja.Object) error {
	if err := defineWebConstructorObject(a.runtime, domExceptionObj, "DOMException", 0); err != nil {
		return err
	}
	prototype, _ := domExceptionObj.Get("prototype").(*goja.Object)
	if prototype == nil {
		return fmt.Errorf("DOMException prototype not found")
	}
	a.domExceptionPrototype = prototype
	if errorPrototype == nil {
		return fmt.Errorf("Error prototype not found")
	}
	if err := prototype.SetPrototype(errorPrototype); err != nil {
		return fmt.Errorf("set DOMException Error prototype: %w", err)
	}

	getter := func(name string, value func(*domExceptionWrapper) goja.Value) error {
		return defineWebAccessor(a.runtime, prototype, name, true, func(call goja.FunctionCall) goja.Value {
			return value(a.domExceptionValue(call.This))
		}, nil)
	}
	if err := getter("message", func(state *domExceptionWrapper) goja.Value { return a.runtime.ToValue(state.message) }); err != nil {
		return fmt.Errorf("define DOMException message: %w", err)
	}
	if err := getter("name", func(state *domExceptionWrapper) goja.Value { return a.runtime.ToValue(state.name) }); err != nil {
		return fmt.Errorf("define DOMException name: %w", err)
	}
	if err := getter("code", func(state *domExceptionWrapper) goja.Value { return a.runtime.ToValue(state.code) }); err != nil {
		return fmt.Errorf("define DOMException code: %w", err)
	}

	if err := defineWebTag(a.runtime, prototype, "DOMException"); err != nil {
		return err
	}

	constants := []struct {
		name  string
		value int
	}{
		{"INDEX_SIZE_ERR", DOMExceptionIndexSizeErr},
		{"DOMSTRING_SIZE_ERR", DOMExceptionDOMStringSizeErr},
		{"HIERARCHY_REQUEST_ERR", DOMExceptionHierarchyRequestErr},
		{"WRONG_DOCUMENT_ERR", DOMExceptionWrongDocumentErr},
		{"INVALID_CHARACTER_ERR", DOMExceptionInvalidCharacterErr},
		{"NO_DATA_ALLOWED_ERR", DOMExceptionNoDataAllowedErr},
		{"NO_MODIFICATION_ALLOWED_ERR", DOMExceptionNoModificationAllowedErr},
		{"NOT_FOUND_ERR", DOMExceptionNotFoundErr},
		{"NOT_SUPPORTED_ERR", DOMExceptionNotSupportedErr},
		{"INUSE_ATTRIBUTE_ERR", DOMExceptionInUseAttributeErr},
		{"INVALID_STATE_ERR", DOMExceptionInvalidStateErr},
		{"SYNTAX_ERR", DOMExceptionSyntaxErr},
		{"INVALID_MODIFICATION_ERR", DOMExceptionInvalidModificationErr},
		{"NAMESPACE_ERR", DOMExceptionNamespaceErr},
		{"INVALID_ACCESS_ERR", DOMExceptionInvalidAccessErr},
		{"VALIDATION_ERR", DOMExceptionValidationErr},
		{"TYPE_MISMATCH_ERR", DOMExceptionTypeMismatchErr},
		{"SECURITY_ERR", DOMExceptionSecurityErr},
		{"NETWORK_ERR", DOMExceptionNetworkErr},
		{"ABORT_ERR", DOMExceptionAbortErr},
		{"URL_MISMATCH_ERR", DOMExceptionURLMismatchErr},
		{"QUOTA_EXCEEDED_ERR", DOMExceptionQuotaExceededErr},
		{"TIMEOUT_ERR", DOMExceptionTimeoutErr},
		{"INVALID_NODE_TYPE_ERR", DOMExceptionInvalidNodeTypeErr},
		{"DATA_CLONE_ERR", DOMExceptionDataCloneErr},
	}
	for _, constant := range constants {
		value := a.runtime.ToValue(constant.value)
		if err := domExceptionObj.DefineDataProperty(constant.name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return fmt.Errorf("define DOMException.%s: %w", constant.name, err)
		}
		if err := prototype.DefineDataProperty(constant.name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return fmt.Errorf("define DOMException.prototype.%s: %w", constant.name, err)
		}
	}

	return nil
}
