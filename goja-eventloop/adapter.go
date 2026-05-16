package gojaeventloop

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	"github.com/joeycumines/logiface"
)

var (
	gojaArrayBufferReflectType = reflect.TypeFor[goja.ArrayBuffer]()
	gojaPromiseReflectType     = reflect.TypeFor[*goja.Promise]()
	gojaProxyReflectType       = reflect.TypeFor[goja.Proxy]()
)

// Adapter bridges one Goja runtime to one event loop. Adapter values must not
// be copied after construction; use the pointer returned by [New].
//
// This allows setTimeout/setInterval/queueMicrotask/Promise to work with Goja.
type Adapter struct { //nolint:govet // betteralign:ignore
	// dispatchJSEvents maps Go Event pointers to their JS wrapper objects during dispatch.
	// This allows event listeners to receive the original JS event object (including CustomEvent.detail).
	dispatchJSEvents sync.Map // map[*goeventloop.Event]goja.Value

	consoleTimersMu   sync.RWMutex // protects consoleTimers
	consoleOutputMu   sync.RWMutex // protects consoleOutput
	consoleCountersMu sync.RWMutex // protects consoleCounters
	consoleIndentMu   sync.RWMutex // protects consoleIndent
	timersMu          sync.Mutex   // protects timers and nextTimerID
	immediatesMu      sync.Mutex   // protects immediates and immediate queue state
	processMu         sync.Mutex   // protects process rejection state

	js                       *goeventloop.JS
	runtime                  *goja.Runtime
	loop                     *goeventloop.Loop
	ownership                *adapterOwnership
	processObj               *goja.Object
	processEmitterCore       *processEmitterCore
	performance              *goeventloop.Performance
	eventTimeSource          goja.Callable
	eventTimeReceiver        goja.Value
	timerClockOrigin         time.Time
	consoleTimers            map[string]time.Time // label -> start time
	consoleCounters          map[string]int       // label -> count
	timers                   map[uint64]*adapterTimer
	timerRegistry            *adapterTimerRegistry
	timerBackendWake         *timerBackendWake
	timerBackendHooks        *timerBackendTestHooks
	delayHead                *adapterDelayState
	timeoutBackendRefed      bool
	genericImmediateRefID    uint64
	genericImmediateRefs     int
	immediates               map[uint64]*adapterImmediate
	pendingRejections        map[*goja.Promise]goja.Value
	pendingRejectionOrder    []*goja.Promise
	reportedRejectionSet     *goja.Object
	rejectionIDStore         *goja.Object
	rejectionWarningStore    *goja.Object
	weakSetAdd               goja.Callable
	weakSetHas               goja.Callable
	weakSetDelete            goja.Callable
	weakMapGet               goja.Callable
	weakMapSet               goja.Callable
	getIterator              goja.Callable // Helper function to get [Symbol.iterator]
	functionToString         goja.Callable // Captured Function.prototype.toString.
	consoleOutput            io.Writer     // output writer for console (defaults to os.Stderr)
	entropyReader            io.Reader     // test seam; nil selects crypto/rand.Reader
	timerStateStore          *goja.Object
	immediateStateStore      *goja.Object
	genericImmediateStore    *goja.Object
	eventTargetStateStore    *goja.Object
	eventStateStore          *goja.Object
	uncloneableStateStore    *goja.Object
	eventPrototype           *goja.Object
	eventIsTrustedGetter     goja.Value
	domExceptionStateStore   *goja.Object
	domExceptionPrototype    *goja.Object
	abortSignalStateStore    *goja.Object
	abortSignalPrototype     *goja.Object
	abortControllerStore     *goja.Object
	disposeSymbol            *goja.Symbol
	timerStateSymbol         *goja.Symbol
	immediateStateSymbol     *goja.Symbol
	eventTargetStateSymbol   *goja.Symbol
	eventStateSymbol         *goja.Symbol
	abortSignalStateSymbol   *goja.Symbol
	abortControllerSymbol    *goja.Symbol
	timeoutPrototype         *goja.Object
	immediatePrototype       *goja.Object
	timeoutInitializer       goja.Callable
	immediateInitializer     goja.Callable
	timerCallbackRunner      goja.Callable
	timerActivator           goja.Callable
	timerProcessor           goja.Callable
	timerTerminator          goja.Callable
	timerSnapshot            goja.Callable
	immediateInvoker         goja.Callable
	immediateCycleEnsurer    goja.Callable
	timerDelayCoercer        goja.Callable
	clearTimeoutFunction     goja.Value
	clearIntervalFunction    goja.Value
	clearImmediateFunction   goja.Value
	errorConstructor         *goja.Object
	rangeErrorConstructor    goja.Callable
	objectGetPrototypeOf     goja.Callable
	objectGetOwnPropertyDesc goja.Callable
	ordinaryFunctionFactory  goja.Callable
	processClone             goja.Callable
	propertyRestore          goja.Callable
	webConstructorFactory    goja.Callable
	structuredCloneBrands    *structuredCloneIntrinsics
	consoleIndent            int // current group indentation level
	nextTimerID              uint64
	nextImmediateID          uint64
	nextRejectionID          uint64
	rejectionCheckScheduled  bool
	exiting                  atomic.Bool
	exitEmitted              atomic.Bool
	suppressBeforeExit       atomic.Bool
	exitCodeSet              atomic.Bool
	exitCode                 atomic.Int64
	warnedNegativeDelay      atomic.Bool
	warnedNaNDelay           atomic.Bool
}

const maxTimerID = 1<<53 - 1

type adapterTimerKind uint8

const (
	adapterTimerTimeout adapterTimerKind = iota + 1
	adapterTimerInterval
	adapterTimerAbort
)

type adapterTimer struct {
	payload    atomic.Pointer[adapterTimerPayload]
	id         uint64
	canceled   atomic.Bool
	executing  atomic.Bool
	active     atomic.Bool
	refed      atomic.Bool
	refKnown   atomic.Bool
	cleanupSet atomic.Bool
	kind       adapterTimerKind
}

type adapterTimerPayload struct {
	object *goja.Object
}

type adapterImmediate struct {
	callbackValue     goja.Value
	argumentSet       goja.Value
	object            *goja.Object
	callback          goja.Callable
	args              []goja.Value
	id                uint64
	canceled          atomic.Bool
	refed             atomic.Bool
	initializing      atomic.Bool
	counted           atomic.Bool
	initializerFailed atomic.Bool
	corePending       atomic.Bool
	carrierHeld       atomic.Bool
}

type genericImmediateState struct {
	held atomic.Bool
}

type structuredCloneIntrinsics struct {
	constructors         map[string]goja.Constructor
	dateGetTime          goja.Callable
	regexpSource         goja.Callable
	regexpGlobal         goja.Callable
	regexpIgnoreCase     goja.Callable
	regexpMultiline      goja.Callable
	regexpDotAll         goja.Callable
	regexpUnicode        goja.Callable
	regexpSticky         goja.Callable
	dataViewBuffer       goja.Callable
	dataViewByteOffset   goja.Callable
	dataViewByteLength   goja.Callable
	typedArrayBuffer     goja.Callable
	typedArrayByteOffset goja.Callable
	typedArrayLength     goja.Callable
	typedArrayName       goja.Callable
	mapSize              goja.Callable
	mapForEach           goja.Callable
	mapSet               goja.Callable
	setSize              goja.Callable
	setForEach           goja.Callable
	setAdd               goja.Callable
	weakMapHas           goja.Callable
	weakSetHas           goja.Callable
	booleanValueOf       goja.Callable
	numberValueOf        goja.Callable
	bigintValueOf        goja.Callable
	stringValueOf        goja.Callable
	symbolValueOf        goja.Callable
}

type timerDelayCoercion struct {
	message     string
	name        string
	idleTimeout float64
	delay       int
}

type promiseJobErrorReporter func(error)

type processExitSignal struct {
	code int
}

func (s processExitSignal) Error() string {
	return fmt.Sprintf("goja-eventloop: process.exit(%d)", s.code)
}

// newPromiseJobEnqueuer returns a [goja.PromiseJobEnqueuer] that routes
// goja's native promise jobs (async/await continuations, native Promise
// reactions) to the event loop's microtask queue. Each job is wrapped in a
// closure that calls [goja.Runtime.RunPromiseJob] for uncatchable-exception
// recovery, then scheduled via [goeventloop.Loop.ScheduleMicrotask] which
// runs it under the loop's serialized logical callback owner in FIFO order.
//
// Extracted from [New] as a package-level function so the closures get a
// meaningful prefix in stack traces (newPromiseJobEnqueuer.func1) instead
// of New.func1.
func newPromiseJobEnqueuer(loop *goeventloop.Loop, runtime *goja.Runtime, report promiseJobErrorReporter) goja.PromiseJobEnqueuer {
	return newPromiseJobEnqueuerWithGate(loop, runtime, report, nil)
}

func newPromiseJobEnqueuerWithGate(loop *goeventloop.Loop, runtime *goja.Runtime, report promiseJobErrorReporter, exiting func() bool) goja.PromiseJobEnqueuer {
	if report == nil {
		report = defaultPromiseJobErrorReporter(loop)
	}
	return func(job func()) {
		if exiting != nil && exiting() {
			return
		}
		if err := loop.ScheduleMicrotask(func() {
			if exiting != nil && exiting() {
				return
			}
			if err := runtime.RunPromiseJob(job); err != nil {
				report(wrapRuntimeError("run promise job", err))
			}
		}); err != nil {
			if exiting != nil && errors.Is(err, goeventloop.ErrLoopTerminated) {
				return
			}
			report(fmt.Errorf("goja-eventloop: enqueue promise job: %w", err))
		}
	}
}

func defaultPromiseJobErrorReporter(loop *goeventloop.Loop) promiseJobErrorReporter {
	return func(err error) {
		reportPromiseJobError(loop, err)
	}
}

func errorConstructor(runtime *goja.Runtime) *goja.Object {
	if runtime == nil {
		return nil
	}
	value, _ := runtime.Intrinsic(goja.IntrinsicErrorConstructor)
	constructor, _ := value.(*goja.Object)
	return constructor
}

func rangeErrorConstructor(runtime *goja.Runtime) goja.Callable {
	if runtime == nil {
		return nil
	}
	value, _ := runtime.Intrinsic(goja.IntrinsicRangeErrorConstructor)
	constructor, _ := goja.AssertFunction(value)
	return constructor
}

func objectGetPrototypeOf(runtime *goja.Runtime) goja.Callable {
	if runtime == nil {
		return nil
	}
	value, _ := runtime.Intrinsic(goja.IntrinsicObjectGetPrototypeOf)
	fn, _ := goja.AssertFunction(value)
	return fn
}

func objectGetOwnPropertyDescriptor(runtime *goja.Runtime) goja.Callable {
	if runtime == nil {
		return nil
	}
	value, _ := runtime.Intrinsic(goja.IntrinsicObjectGetOwnPropertyDescriptor)
	fn, _ := goja.AssertFunction(value)
	return fn
}

func runtimeIntrinsic(runtime *goja.Runtime, id goja.Intrinsic, name string) (goja.Value, error) {
	if runtime == nil {
		return nil, fmt.Errorf("goja-eventloop: %s intrinsic runtime is unavailable", name)
	}
	value, ok := runtime.Intrinsic(id)
	if !ok || value == nil {
		return nil, fmt.Errorf("goja-eventloop: %s intrinsic is unavailable", name)
	}
	return value, nil
}

func runtimeIntrinsicObject(runtime *goja.Runtime, id goja.Intrinsic, name string) (*goja.Object, error) {
	value, err := runtimeIntrinsic(runtime, id, name)
	if err != nil {
		return nil, err
	}
	object, ok := value.(*goja.Object)
	if !ok || object == nil {
		return nil, fmt.Errorf("goja-eventloop: %s intrinsic is not an object", name)
	}
	return object, nil
}

func runtimeIntrinsicCallable(runtime *goja.Runtime, id goja.Intrinsic, name string) (goja.Callable, error) {
	value, err := runtimeIntrinsic(runtime, id, name)
	if err != nil {
		return nil, err
	}
	callable, ok := goja.AssertFunction(value)
	if !ok {
		return nil, fmt.Errorf("goja-eventloop: %s intrinsic is not callable", name)
	}
	return callable, nil
}

func newStructuredCloneIntrinsics(runtime *goja.Runtime) (*structuredCloneIntrinsics, error) {
	if runtime == nil {
		return nil, fmt.Errorf("runtime cannot be nil")
	}
	intrinsics := &structuredCloneIntrinsics{
		constructors: make(map[string]goja.Constructor),
	}
	for _, spec := range []struct {
		name string
		id   goja.Intrinsic
	}{
		{name: "Date", id: goja.IntrinsicDateConstructor},
		{name: "RegExp", id: goja.IntrinsicRegExpConstructor},
		{name: "Map", id: goja.IntrinsicMapConstructor},
		{name: "Set", id: goja.IntrinsicSetConstructor},
		{name: "DataView", id: goja.IntrinsicDataViewConstructor},
		{name: "Error", id: goja.IntrinsicErrorConstructor},
		{name: "EvalError", id: goja.IntrinsicEvalErrorConstructor},
		{name: "RangeError", id: goja.IntrinsicRangeErrorConstructor},
		{name: "ReferenceError", id: goja.IntrinsicReferenceErrorConstructor},
		{name: "SyntaxError", id: goja.IntrinsicSyntaxErrorConstructor},
		{name: "TypeError", id: goja.IntrinsicTypeErrorConstructor},
		{name: "URIError", id: goja.IntrinsicURIErrorConstructor},
		{name: "Int8Array", id: goja.IntrinsicInt8ArrayConstructor},
		{name: "Uint8Array", id: goja.IntrinsicUint8ArrayConstructor},
		{name: "Uint8ClampedArray", id: goja.IntrinsicUint8ClampedArrayConstructor},
		{name: "Int16Array", id: goja.IntrinsicInt16ArrayConstructor},
		{name: "Uint16Array", id: goja.IntrinsicUint16ArrayConstructor},
		{name: "Int32Array", id: goja.IntrinsicInt32ArrayConstructor},
		{name: "Uint32Array", id: goja.IntrinsicUint32ArrayConstructor},
		{name: "Float32Array", id: goja.IntrinsicFloat32ArrayConstructor},
		{name: "Float64Array", id: goja.IntrinsicFloat64ArrayConstructor},
		{name: "BigInt64Array", id: goja.IntrinsicBigInt64ArrayConstructor},
		{name: "BigUint64Array", id: goja.IntrinsicBigUint64ArrayConstructor},
	} {
		value, ok := runtime.Intrinsic(spec.id)
		if !ok {
			return nil, fmt.Errorf("failed to initialize structuredClone intrinsics: %s is unavailable", spec.name)
		}
		constructor, ok := goja.AssertConstructor(value)
		if !ok {
			return nil, fmt.Errorf("failed to initialize structuredClone intrinsics: %s is not a constructor", spec.name)
		}
		intrinsics.constructors[spec.name] = constructor
	}
	for _, spec := range []struct {
		name   string
		id     goja.Intrinsic
		target *goja.Callable
	}{
		{name: "Date.prototype.getTime", id: goja.IntrinsicDateGetTime, target: &intrinsics.dateGetTime},
		{name: "RegExp.prototype.source", id: goja.IntrinsicRegExpSourceGetter, target: &intrinsics.regexpSource},
		{name: "RegExp.prototype.global", id: goja.IntrinsicRegExpGlobalGetter, target: &intrinsics.regexpGlobal},
		{name: "RegExp.prototype.ignoreCase", id: goja.IntrinsicRegExpIgnoreCaseGetter, target: &intrinsics.regexpIgnoreCase},
		{name: "RegExp.prototype.multiline", id: goja.IntrinsicRegExpMultilineGetter, target: &intrinsics.regexpMultiline},
		{name: "RegExp.prototype.dotAll", id: goja.IntrinsicRegExpDotAllGetter, target: &intrinsics.regexpDotAll},
		{name: "RegExp.prototype.unicode", id: goja.IntrinsicRegExpUnicodeGetter, target: &intrinsics.regexpUnicode},
		{name: "RegExp.prototype.sticky", id: goja.IntrinsicRegExpStickyGetter, target: &intrinsics.regexpSticky},
		{name: "DataView.prototype.buffer", id: goja.IntrinsicDataViewBufferGetter, target: &intrinsics.dataViewBuffer},
		{name: "DataView.prototype.byteOffset", id: goja.IntrinsicDataViewByteOffsetGetter, target: &intrinsics.dataViewByteOffset},
		{name: "DataView.prototype.byteLength", id: goja.IntrinsicDataViewByteLengthGetter, target: &intrinsics.dataViewByteLength},
		{name: "TypedArray.prototype.buffer", id: goja.IntrinsicTypedArrayBufferGetter, target: &intrinsics.typedArrayBuffer},
		{name: "TypedArray.prototype.byteOffset", id: goja.IntrinsicTypedArrayByteOffsetGetter, target: &intrinsics.typedArrayByteOffset},
		{name: "TypedArray.prototype.length", id: goja.IntrinsicTypedArrayLengthGetter, target: &intrinsics.typedArrayLength},
		{name: "TypedArray.prototype[Symbol.toStringTag]", id: goja.IntrinsicTypedArrayNameGetter, target: &intrinsics.typedArrayName},
		{name: "Map.prototype.size", id: goja.IntrinsicMapSizeGetter, target: &intrinsics.mapSize},
		{name: "Map.prototype.forEach", id: goja.IntrinsicMapForEach, target: &intrinsics.mapForEach},
		{name: "Map.prototype.set", id: goja.IntrinsicMapSet, target: &intrinsics.mapSet},
		{name: "Set.prototype.size", id: goja.IntrinsicSetSizeGetter, target: &intrinsics.setSize},
		{name: "Set.prototype.forEach", id: goja.IntrinsicSetForEach, target: &intrinsics.setForEach},
		{name: "Set.prototype.add", id: goja.IntrinsicSetAdd, target: &intrinsics.setAdd},
		{name: "WeakMap.prototype.has", id: goja.IntrinsicWeakMapHas, target: &intrinsics.weakMapHas},
		{name: "WeakSet.prototype.has", id: goja.IntrinsicWeakSetHas, target: &intrinsics.weakSetHas},
		{name: "Boolean.prototype.valueOf", id: goja.IntrinsicBooleanValueOf, target: &intrinsics.booleanValueOf},
		{name: "Number.prototype.valueOf", id: goja.IntrinsicNumberValueOf, target: &intrinsics.numberValueOf},
		{name: "BigInt.prototype.valueOf", id: goja.IntrinsicBigIntValueOf, target: &intrinsics.bigintValueOf},
		{name: "String.prototype.valueOf", id: goja.IntrinsicStringValueOf, target: &intrinsics.stringValueOf},
		{name: "Symbol.prototype.valueOf", id: goja.IntrinsicSymbolValueOf, target: &intrinsics.symbolValueOf},
	} {
		value, ok := runtime.Intrinsic(spec.id)
		if !ok {
			return nil, fmt.Errorf("failed to initialize structuredClone intrinsics: %s is unavailable", spec.name)
		}
		callable, ok := goja.AssertFunction(value)
		if !ok {
			return nil, fmt.Errorf("failed to initialize structuredClone intrinsics: %s is not callable", spec.name)
		}
		*spec.target = callable
	}
	return intrinsics, nil
}

// New creates a new Goja adapter for given event loop and runtime.
//
// New reserves the exact loop/runtime ownership pair and prepares only detached
// private values. [Adapter.Bind] installs Promise hooks and the retained global
// surface after its reversible installation transaction succeeds.
//
// jsOptions are forwarded to [goeventloop.BindJS] for the underlying JavaScript
// timer/promise adapter. Use these to configure options provided by this
// module's declared go-eventloop dependency before binding the Goja runtime.
// New returns an error if loop or runtime is nil or a forwarded JS option
// violates its documented contract. Dynamic ownership and
// runtime-initialization failures are also returned as errors.
func New(loop *goeventloop.Loop, runtime *goja.Runtime, jsOptions ...goeventloop.JSOption) (*Adapter, error) {
	if loop == nil {
		return nil, errors.New("goja-eventloop: loop must not be nil")
	}
	if runtime == nil {
		return nil, errors.New("goja-eventloop: runtime must not be nil")
	}
	gojaSafeOptions := append([]goeventloop.JSOption{goeventloop.WithUnhandledRejectionFallback(goeventloop.UnhandledRejectionFallbackDisabled)}, jsOptions...)
	if err := goeventloop.ValidateJSOptions(gojaSafeOptions...); err != nil {
		return nil, fmt.Errorf("goja-eventloop: validate JS options: %w", err)
	}

	adapter := &Adapter{
		runtime:           runtime,
		loop:              loop,
		consoleTimers:     make(map[string]time.Time),
		consoleCounters:   make(map[string]int),
		timers:            make(map[uint64]*adapterTimer),
		timerRegistry:     newAdapterTimerRegistry(),
		timerClockOrigin:  time.Now(),
		immediates:        make(map[uint64]*adapterImmediate),
		pendingRejections: make(map[*goja.Promise]goja.Value),
		consoleOutput:     os.Stderr, // Default to stderr like browsers/Node.js
	}
	if err := claimAdapter(adapter, gojaSafeOptions); err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			adapter.fail()
		}
	}()
	var initializationErr error
	if exception := runtime.Try(func() { initializationErr = adapter.initialize() }); exception != nil {
		return nil, wrapRuntimeException("initialize runtime", exception)
	}
	if initializationErr != nil {
		return nil, initializationErr
	}

	succeeded = true
	return adapter, nil
}

func (a *Adapter) initialize() error {
	a.errorConstructor = errorConstructor(a.runtime)
	a.rangeErrorConstructor = rangeErrorConstructor(a.runtime)
	a.objectGetPrototypeOf = objectGetPrototypeOf(a.runtime)
	a.objectGetOwnPropertyDesc = objectGetOwnPropertyDescriptor(a.runtime)
	a.timerStateSymbol = goja.NewSymbol("goja-eventloop.timerState")
	a.immediateStateSymbol = goja.NewSymbol("goja-eventloop.immediateState")
	a.eventTargetStateSymbol = goja.NewSymbol("goja-eventloop.eventTargetState")
	a.eventStateSymbol = goja.NewSymbol("goja-eventloop.eventState")
	a.abortSignalStateSymbol = goja.NewSymbol("goja-eventloop.abortSignalState")
	a.abortControllerSymbol = goja.NewSymbol("goja-eventloop.abortControllerState")
	if err := a.initInstallationHelpers(); err != nil {
		return err
	}
	processEmitter, err := a.newProcessEmitterCore()
	if err != nil {
		return err
	}
	a.processEmitterCore = processEmitter
	structuredCloneBrands, err := newStructuredCloneIntrinsics(a.runtime)
	if err != nil {
		return err
	}
	a.structuredCloneBrands = structuredCloneBrands
	if err := a.initHiddenStateStores(); err != nil {
		return err
	}
	if err := a.initHandlePrototypes(); err != nil {
		return err
	}
	return a.initRejectionWeakSet()
}

func (a *Adapter) initHiddenStateStores() error {
	constructor, err := runtimeIntrinsic(a.runtime, goja.IntrinsicWeakMapConstructor, "WeakMap")
	if err != nil {
		return err
	}
	a.weakMapGet, err = runtimeIntrinsicCallable(a.runtime, goja.IntrinsicWeakMapGet, "WeakMap.prototype.get")
	if err != nil {
		return err
	}
	a.weakMapSet, err = runtimeIntrinsicCallable(a.runtime, goja.IntrinsicWeakMapSet, "WeakMap.prototype.set")
	if err != nil {
		return err
	}
	store := func(name string) (*goja.Object, error) {
		object, err := a.runtime.New(constructor)
		if err != nil {
			return nil, wrapRuntimeError("initialize "+name+" hidden state store", err)
		}
		return object, nil
	}
	if a.timerStateStore, err = store("timer"); err != nil {
		return err
	}
	if a.immediateStateStore, err = store("immediate"); err != nil {
		return err
	}
	if a.genericImmediateStore, err = store("genericImmediate"); err != nil {
		return err
	}
	if a.eventTargetStateStore, err = store("eventTarget"); err != nil {
		return err
	}
	if a.eventStateStore, err = store("event"); err != nil {
		return err
	}
	if a.uncloneableStateStore, err = store("uncloneable"); err != nil {
		return err
	}
	if a.domExceptionStateStore, err = store("domException"); err != nil {
		return err
	}
	if a.abortSignalStateStore, err = store("abortSignal"); err != nil {
		return err
	}
	if a.abortControllerStore, err = store("abortController"); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) setHiddenState(store *goja.Object, key *goja.Object, state any) {
	if a == nil || store == nil || key == nil || a.weakMapSet == nil {
		panic("goja-eventloop: hidden state store is not initialized")
	}
	if _, err := a.weakMapSet(store, key, a.runtime.ToValue(state)); err != nil {
		a.panicJSException(err)
	}
}

func (a *Adapter) hiddenState(store *goja.Object, key *goja.Object) goja.Value {
	if a == nil || store == nil || key == nil || a.weakMapGet == nil {
		return goja.Undefined()
	}
	value, err := a.weakMapGet(store, key)
	if err != nil {
		a.panicJSException(err)
	}
	if value == nil {
		return goja.Undefined()
	}
	return value
}

func (a *Adapter) reportPromiseJobError(err error) {
	err = wrapRuntimeExceptionError("report promise job", err)
	if _, ok := processExitCode(err); ok {
		return
	}
	reportPromiseJobError(a.loop, err)
}

func reportPromiseJobError(loop *goeventloop.Loop, err error) {
	if err == nil {
		return
	}
	err = wrapRuntimeExceptionError("report promise job", err)
	logStructuredAdapterError(loop, func(event logiface.Event) {
		addAdapterLogString(event, "component", "goja-eventloop")
		addAdapterLogError(event, err)
		addAdapterLogMessage(event, "goja native promise job failed")
	})
}

func (a *Adapter) initRejectionWeakSet() error {
	weakSet, err := runtimeIntrinsic(a.runtime, goja.IntrinsicWeakSetConstructor, "WeakSet")
	if err != nil {
		return err
	}
	weakMap, err := runtimeIntrinsic(a.runtime, goja.IntrinsicWeakMapConstructor, "WeakMap")
	if err != nil {
		return err
	}
	if a.reportedRejectionSet, err = a.runtime.New(weakSet); err != nil {
		return wrapRuntimeError("initialize rejection WeakSet", err)
	}
	if a.rejectionIDStore, err = a.runtime.New(weakMap); err != nil {
		return wrapRuntimeError("initialize rejection ID store", err)
	}
	if a.rejectionWarningStore, err = a.runtime.New(weakMap); err != nil {
		return wrapRuntimeError("initialize rejection warning store", err)
	}
	a.weakSetAdd, err = runtimeIntrinsicCallable(a.runtime, goja.IntrinsicWeakSetAdd, "WeakSet.prototype.add")
	if err != nil {
		return err
	}
	a.weakSetHas, err = runtimeIntrinsicCallable(a.runtime, goja.IntrinsicWeakSetHas, "WeakSet.prototype.has")
	if err != nil {
		return err
	}
	a.weakSetDelete, err = runtimeIntrinsicCallable(a.runtime, goja.IntrinsicWeakSetDelete, "WeakSet.prototype.delete")
	if err != nil {
		return err
	}
	return nil
}

func logStructuredAdapterError(loop *goeventloop.Loop, write func(logiface.Event)) {
	loop.Log(logiface.LevelError, logiface.ModifierFunc[logiface.Event](func(event logiface.Event) error {
		write(event)
		return nil
	}))
}

func addAdapterLogString(event logiface.Event, key, value string) {
	if !event.AddString(key, value) {
		event.AddField(key, value)
	}
}

func addAdapterLogError(event logiface.Event, err error) {
	if !event.AddError(err) {
		event.AddField("err", err)
	}
}

func addAdapterLogMessage(event logiface.Event, message string) {
	if !event.AddMessage(message) {
		event.AddField("msg", message)
	}
}
