package gojaeventloop

// timerListFactorySource retains the ordering and list-generation algorithms
// from Node.js v26.5.0's lib/internal/timers.js. JavaScript owns every
// user-visible property access and every TimersList/PriorityQueue mutation.
// Go supplies only the single native wake carrier and callback boundary.
const timerListFactorySource = `
((createTimeout, createImmediate, nativeSchedule, nativeToggleRef,
	  nativeInserted, nativeRetired, nativeStateRef, nativeRunTimer, nativeNow,
	  nativeImmediateRef, nativeImmediateClear, nativeImmediateCounted,
	  nativeRunCheckpoint, disposeSymbol,
	  apply, mathMax, mathTrunc, iteratorSymbol, toPrimitiveSymbol, stringConvert,
	  TypeErrorConstructor, kRefed, kHasPrimitive, asyncId, triggerId,
	  kAsyncContextFrame) => {
  "use strict";
  const knownTimersById = { __proto__: null };
  const timerListMap = { __proto__: null };
  let timerListId = -9007199254740991;
  let timerListCount = 0;
  let nextAsyncId = 0;
  let nextExpiry = Infinity;
  let timeoutRefCount = 0;
	  let immediateHead = null;
	  let immediateTail = null;
	  let immediateOutstanding = null;
	  let immediatePrevious = null;
	  let immediatePublished = false;

  function initAsyncResource(resource) {
    resource[asyncId] = ++nextAsyncId;
    resource[triggerId] = 1;
    resource[kAsyncContextFrame] = undefined;
  }

  function compareTimersLists(a, b) {
    const expiryDiff = a.expiry - b.expiry;
    if (expiryDiff === 0) return a.id - b.id;
    return expiryDiff;
  }

  class PriorityQueue {
    constructor(compare, setPosition) {
      this.heap = [undefined];
      this.size = 0;
      this.compare = compare;
      this.setPosition = setPosition;
    }
    peek() {
      return this.size > 0 ? this.heap[1] : undefined;
    }
    shift() {
      const value = this.peek();
      if (value !== undefined) this.removeAt(1);
      return value;
    }
    clear() {
      this.heap = [undefined];
      this.size = 0;
    }
    insert(value) {
      const position = ++this.size;
      this.heap[position] = value;
      this.percolateUp(position);
    }
    percolateUp(position) {
      const value = this.heap[position];
      while (position > 1) {
        const parent = position >>> 1;
        const parentValue = this.heap[parent];
        if (this.compare(parentValue, value) <= 0) break;
        this.heap[position] = parentValue;
        this.setPosition(parentValue, position);
        position = parent;
      }
      this.heap[position] = value;
      this.setPosition(value, position);
    }
    percolateDown(position) {
      const value = this.heap[position];
      const size = this.size;
      for (;;) {
        const left = position << 1;
        if (left > size) break;
        const right = left + 1;
        let child = left;
        let childValue = this.heap[left];
        if (right <= size && this.compare(this.heap[right], childValue) < 0) {
          child = right;
          childValue = this.heap[right];
        }
        if (this.compare(value, childValue) <= 0) break;
        this.setPosition(childValue, position);
        this.heap[position] = childValue;
        position = child;
      }
      this.heap[position] = value;
      this.setPosition(value, position);
    }
    removeAt(position) {
      let size = this.size;
      this.heap[position] = this.heap[size];
      this.heap[size] = undefined;
      size = --this.size;
      if (size > 0 && position <= size) {
        if (position > 1 &&
            this.compare(this.heap[position >>> 1], this.heap[position]) > 0) {
          this.percolateUp(position);
        } else {
          this.percolateDown(position);
        }
      }
    }
  }

  const timerListQueue = new PriorityQueue(
    compareTimersLists,
    (list, position) => { list.priorityQueuePosition = position; },
  );

  class TimersList {
    constructor(expiry, msecs) {
      this._idleNext = this;
      this._idlePrev = this;
      this.expiry = expiry;
      this.id = timerListId++;
      this.msecs = msecs;
      this.priorityQueuePosition = null;
    }
  }

  function remove(item) {
    if (item._idleNext) item._idleNext._idlePrev = item._idlePrev;
    if (item._idlePrev) item._idlePrev._idleNext = item._idleNext;
    item._idleNext = null;
    item._idlePrev = null;
  }

  function append(list, item) {
    if (item._idleNext || item._idlePrev) remove(item);
    item._idleNext = list._idleNext;
    item._idlePrev = list;
    list._idleNext._idlePrev = item;
    list._idleNext = item;
  }

  function peek(list) {
    return list._idlePrev === list ? null : list._idlePrev;
  }

  function incRefCount() {
    const previous = timeoutRefCount;
    timeoutRefCount = (previous + 1) | 0;
    if (previous === 0) nativeToggleRef(true);
  }

	  function decRefCount() {
	    timeoutRefCount = (timeoutRefCount - 1) | 0;
	    if (timeoutRefCount === 0) nativeToggleRef(false);
	  }

	  function decRefCountProcessing() {
	    timeoutRefCount = (timeoutRefCount - 1) | 0;
	  }

  function insert(item, msecs, start = nativeNow()) {
    msecs = mathTrunc(msecs);
    item._idleStart = start;
    let list = timerListMap[msecs];
    if (list === undefined) {
      const expiry = start + msecs;
      list = new TimersList(expiry, msecs);
      timerListMap[msecs] = list;
      timerListCount++;
      timerListQueue.insert(list);
      if (nextExpiry > expiry) {
        nativeSchedule(msecs);
        nextExpiry = expiry;
      }
    }
    append(list, item);
    nativeInserted(item);
  }

  function insertGuarded(item, refed) {
    const msecs = item._idleTimeout;
    if (msecs < 0 || msecs === undefined) return;
    insert(item, msecs);
    const destroyed = item._destroyed;
    if (destroyed || !item[asyncId]) {
      item._destroyed = false;
      initAsyncResource(item);
    }
    if (destroyed) {
      if (refed) incRefCount();
    } else if (refed === !item[kRefed]) {
      if (refed) incRefCount();
      else decRefCount();
    }
    item[kRefed] = refed;
    nativeStateRef(item, !!refed);
  }

  function destroyTimer(item, retiredAsyncId = item[asyncId]) {
    if (item._destroyed) return;
    item._destroyed = true;
    if (item[kHasPrimitive]) delete knownTimersById[retiredAsyncId];
	    if (item[kRefed]) decRefCountProcessing();
    nativeRetired(item);
  }

  function unenroll(item) {
    if (item._destroyed) return;
    item._destroyed = true;
    if (item[kHasPrimitive]) delete knownTimersById[item[asyncId]];
    void item[asyncId];
    remove(item);
    const refed = item[kRefed];
    if (refed) {
      const msecs = mathTrunc(item._idleTimeout);
      const list = timerListMap[msecs];
      if (list !== undefined && list._idleNext === list) {
        timerListQueue.removeAt(list.priorityQueuePosition);
        delete timerListMap[list.msecs];
        timerListCount--;
      }
      decRefCount();
    }
    item._idleTimeout = -1;
    nativeRetired(item);
  }

  function clearTimer(timer) {
    if (timer !== null && timer !== undefined && timer._onTimeout) {
      timer._onTimeout = null;
      unenroll(timer);
      return;
    }
    if (typeof timer === "number" || typeof timer === "string") {
      const instance = knownTimersById[timer];
      if (instance !== undefined) {
        instance._onTimeout = null;
        unenroll(instance);
      }
    }
  }

  function clearTimeout(timer) { clearTimer(timer); }
  function clearInterval(timer) { clearTimer(timer); }

  class Timeout {
    constructor(callback, after, args, isRepeat, isRefed) {
      return createTimeout(this, callback, after, args, isRepeat, isRefed);
    }
    refresh() {
      if (this[kRefed]) insertGuarded(this, true);
      else insertGuarded(this, false);
      return this;
    }
    unref() {
      if (this[kRefed]) {
        this[kRefed] = false;
        if (!this._destroyed) decRefCount();
        nativeStateRef(this, false);
      }
      return this;
    }
    ref() {
      if (!this[kRefed]) {
        this[kRefed] = true;
        if (!this._destroyed) incRefCount();
        nativeStateRef(this, true);
      }
      return this;
    }
    hasRef() { return this[kRefed]; }
  }

  Timeout.prototype.close = function() {
    clearTimeout(this);
    return this;
  };
  Timeout.prototype[disposeSymbol] = function() { clearTimeout(this); };
  Timeout.prototype[toPrimitiveSymbol] = function() {
    const id = this[asyncId];
    if (!this[kHasPrimitive]) {
      this[kHasPrimitive] = true;
      knownTimersById[id] = this;
    }
    return id;
  };

  function initializeTimeout(handle, callback, idleTimeout, args, repeat, refed) {
    handle._idleTimeout = idleTimeout;
    handle._idlePrev = handle;
    handle._idleNext = handle;
    handle._idleStart = null;
    handle._onTimeout = callback;
    handle._timerArgs = args;
    handle._repeat = repeat;
    handle._destroyed = false;
    if (refed) incRefCount();
    handle[kRefed] = refed;
    handle[kHasPrimitive] = false;
    initAsyncResource(handle);
    nativeStateRef(handle, !!refed);
    return handle;
  }

  function activateTimeout(handle) {
    insert(handle, handle._idleTimeout);
    return handle;
  }

  function runTimerCallback(list, timer, start, retiredAsyncId) {
    let thrown = false;
    let thrownValue;
    try {
      const args = timer._timerArgs;
      if (args === undefined) timer._onTimeout();
      else {
        if (args === null || (typeof args !== "object" && typeof args !== "function")) {
          throw new TypeErrorConstructor("CreateListFromArrayLike called on non-object");
        }
        apply(timer._onTimeout, timer, args);
      }
    } catch (error) {
      thrown = true;
      thrownValue = error;
    } finally {
      if (timer._repeat && timer._idleTimeout !== -1) {
        timer._idleTimeout = timer._repeat;
        insert(timer, timer._idleTimeout, start);
      } else if (!timer._idleNext && !timer._idlePrev && !timer._destroyed) {
        destroyTimer(timer, retiredAsyncId);
      }
    }
    return [thrown, thrownValue];
  }

  function listOnTimeout(list, now) {
    const msecs = list.msecs;
    let ranAtLeastOneTimer = false;
    for (;;) {
      const timer = peek(list);
      if (timer == null) break;
      const diff = now - timer._idleStart;
      if (diff < msecs) {
        list.expiry = mathMax(timer._idleStart + msecs, now + 1);
        list.id = timerListId++;
        timerListQueue.percolateDown(1);
        return true;
      }
      if (ranAtLeastOneTimer) nativeRunCheckpoint();
      else ranAtLeastOneTimer = true;
      remove(timer);
      const retiredAsyncId = timer[asyncId];
      if (!timer._onTimeout) {
        destroyTimer(timer, retiredAsyncId);
        continue;
      }
      void timer[kAsyncContextFrame];
      void timer[triggerId];
      const start = timer._repeat ? nativeNow() : undefined;
      if (!nativeRunTimer(list, timer, start, retiredAsyncId)) return false;
    }
    if (timerListMap[msecs] === list) {
      delete timerListMap[msecs];
      timerListCount--;
      timerListQueue.shift();
    }
    return true;
  }

  function processTimers(now) {
    nextExpiry = Infinity;
    let list;
    let ranAtLeastOneList = false;
    while ((list = timerListQueue.peek()) != null) {
      if (list.expiry > now) {
        nextExpiry = list.expiry;
        return timeoutRefCount > 0 ? nextExpiry : -nextExpiry;
      }
      if (ranAtLeastOneList) nativeRunCheckpoint();
      else ranAtLeastOneList = true;
      if (!listOnTimeout(list, now)) return 0;
    }
    return 0;
  }

  function safely(action) {
    try { action(); } catch {}
  }

  function terminateTimers(handles, immediateHandles) {
    try {
      if (handles !== undefined) {
        for (let index = 0; index < handles.length; index++) {
          const timer = handles[index];
          if (timer === null || timer === undefined) continue;
          safely(() => remove(timer));
          safely(() => { timer._onTimeout = null; });
          safely(() => { timer._timerArgs = null; });
          safely(() => { timer._repeat = null; });
          safely(() => { timer._idleTimeout = -1; });
          safely(() => { timer._destroyed = true; });
          safely(() => nativeRetired(timer));
        }
      }
      if (immediateHandles !== undefined) {
        for (let index = 0; index < immediateHandles.length; index++) {
          const immediate = immediateHandles[index];
          if (immediate === null || immediate === undefined) continue;
          safely(() => { immediate._idleNext = null; });
          safely(() => { immediate._idlePrev = null; });
          safely(() => { immediate._onImmediate = null; });
          safely(() => { immediate._argv = null; });
          safely(() => { immediate._destroyed = true; });
          safely(() => { immediate[kRefed] = null; });
          safely(() => nativeImmediateClear(immediate));
        }
      }
    } finally {
      timerListQueue.clear();
      for (const msecs in timerListMap) delete timerListMap[msecs];
      for (const id in knownTimersById) delete knownTimersById[id];
      timerListCount = 0;
      timeoutRefCount = 0;
      nextExpiry = Infinity;
	      immediateHead = null;
	      immediateTail = null;
	      immediateOutstanding = null;
	      immediatePrevious = null;
	      immediatePublished = false;
	    }
  }

  function timerSnapshot() {
    const list = timerListQueue.peek();
    if (list === undefined) {
      return { timeoutRefCount, listCount: timerListCount, nextExpiry };
    }
    return {
      timeoutRefCount,
      listCount: timerListCount,
      nextExpiry,
      head: {
        _idleNext: list._idleNext === list,
        _idlePrev: list._idlePrev === list,
        expiry: list.expiry,
        id: list.id,
        msecs: list.msecs,
        priorityQueuePosition: list.priorityQueuePosition,
      },
    };
  }

	  function removeImmediate(item) {
	    if (item._idleNext) item._idleNext._idlePrev = item._idlePrev;
	    if (item._idlePrev) item._idlePrev._idleNext = item._idleNext;
	    if (item === immediateHead) immediateHead = item._idleNext;
	    if (item === immediateTail) immediateTail = item._idlePrev;
	    item._idleNext = null;
	    item._idlePrev = null;
	  }

	  function clearImmediate(immediate) {
	    if (immediate === null || immediate === undefined ||
	        !immediate._onImmediate || immediate._destroyed) return;
	    const outstanding = immediate === immediateOutstanding;
	    immediate._destroyed = true;
	    const refed = immediate[kRefed];
	    if (refed) nativeImmediateRef(immediate, false);
	    immediate[kRefed] = null;
	    void immediate[asyncId];
	    immediate._onImmediate = null;
	    removeImmediate(immediate);
	    const resumedHead = outstanding && immediatePrevious === null;
	    if (outstanding && !resumedHead) {
	      immediateOutstanding = immediatePrevious._idleNext;
	    }
	    nativeImmediateClear(immediate, resumedHead);
	  }

  class Immediate {
    constructor(callback, args) { return createImmediate(this, callback, args); }
    ref() {
      if (this[kRefed] === false) {
        this[kRefed] = true;
        nativeImmediateRef(this, true);
      }
      return this;
    }
    unref() {
      if (this[kRefed] === true) {
        this[kRefed] = false;
        nativeImmediateRef(this, false);
      }
      return this;
    }
    hasRef() { return !!this[kRefed]; }
  }
  Immediate.prototype[disposeSymbol] = function() { clearImmediate(this); };

  function initializeImmediate(handle, callback, args) {
    handle._idleNext = null;
    handle._idlePrev = null;
    handle._onImmediate = callback;
    handle._argv = args;
    handle._destroyed = false;
	    handle[kRefed] = false;
	    initAsyncResource(handle);
	    handle.ref();
	    nativeImmediateCounted(handle);
	    if (immediateTail !== null) {
      handle._idlePrev = immediateTail;
      immediateTail._idleNext = handle;
    } else {
      immediateHead = handle;
    }
    immediateTail = handle;
    return handle;
  }

	  function invokeImmediate(handle) {
	    try {
	      if (immediateOutstanding === null) {
	        if (handle !== immediateHead) return [0, undefined];
	        immediateOutstanding = handle;
	        immediatePrevious = null;
	        immediatePublished = false;
	        immediateHead = null;
	        immediateTail = null;
	      }
	      if (handle !== immediateOutstanding) return [0, undefined];
	      if (handle._destroyed) {
	        if (immediatePrevious === null) {
	          throw new TypeErrorConstructor("Cannot read properties of undefined (reading '_idleNext')");
	        }
	        immediateOutstanding = immediatePrevious._idleNext;
	        immediatePublished = immediateOutstanding !== null;
	        return [4, undefined];
	      }
	      handle._destroyed = true;
	      if (handle[kRefed]) nativeImmediateRef(handle, false);
	      handle[kRefed] = null;
	      void handle[kAsyncContextFrame];
	      void handle[asyncId];
	      void handle[triggerId];
	      immediatePrevious = handle;
	    } catch (error) {
	      if (!immediatePublished) immediateOutstanding = null;
	      immediatePrevious = null;
	      return [2, error];
	    }
	    let thrown = false;
	    let thrownValue;
    try {
      const argv = handle._argv;
      if (!argv) handle._onImmediate();
      else {
        const callback = handle._onImmediate;
        const iteratorMethod = argv[iteratorSymbol];
        if (typeof iteratorMethod !== "function") {
          throw new TypeErrorConstructor("Spread syntax requires ...iterable[Symbol.iterator] to be a function");
        }
        const iterator = apply(iteratorMethod, argv, []);
        if (iterator === null || (typeof iterator !== "object" && typeof iterator !== "function")) {
          throw new TypeErrorConstructor("Result of the Symbol.iterator method is not an object");
        }
        const next = iterator.next;
        const values = [];
        for (;;) {
          const step = apply(next, iterator, []);
          if (step === null || (typeof step !== "object" && typeof step !== "function")) {
            throw new TypeErrorConstructor("Iterator result " + stringConvert(step) + " is not an object");
          }
          if (step.done) break;
          values[values.length] = step.value;
        }
        apply(callback, handle, values);
      }
	    } catch (error) {
	      thrown = true;
	      thrownValue = error;
	    }
	    const wasPublished = immediatePublished;
	    try {
	      handle._onImmediate = null;
	      immediateOutstanding = handle._idleNext;
	      immediatePublished = immediateOutstanding !== null;
	    } catch (error) {
	      if (!wasPublished) immediateOutstanding = null;
	      immediatePrevious = null;
	      return [2, error];
	    }
	    if (thrown) immediatePrevious = null;
	    return [thrown ? 3 : 1, thrownValue];
	  }

	  function coerceDelay(value) { return value * 1; }

	  function noop() {}

	  function ensureImmediateCycle() { return new Immediate(noop); }

	  return [
	    Timeout, Immediate, clearTimeout, clearInterval, clearImmediate,
	    initializeTimeout, initializeImmediate, runTimerCallback, activateTimeout,
	    processTimers, terminateTimers, timerSnapshot, invokeImmediate, coerceDelay,
	    ensureImmediateCycle,
	  ];
})
`
