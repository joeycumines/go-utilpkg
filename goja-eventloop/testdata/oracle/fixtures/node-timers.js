async function nodeTimers() {
  function capture(callback) {
    try {
      callback();
      return { ok: true };
    } catch (error) {
      return { ok: false, name: error.name, code: error.code };
    }
  }

  function captureMessage(callback) {
    try {
      callback();
      return { ok: true };
    } catch (error) {
      return { ok: false, name: error.name, message: error.message };
    }
  }

  const immediateSubclassNativeLiveness = await new Promise(function (resolve) {
    const events = [];
    let leaked;
    let refed;
    function beforeExit() {
      process.off("beforeExit", beforeExit);
      if (leaked !== undefined) {
        leaked[refed] = false;
        clearImmediate(leaked);
      }
      resolve(events);
    }
    process.on("beforeExit", beforeExit);
    setTimeout(function () {
      const seed = setImmediate(function () {});
      const Immediate = seed.constructor;
      clearImmediate(seed);
      class ImmediateNativeRefOverride extends Immediate {
        ref() {
          refed = Object.getOwnPropertySymbols(this).find(function (symbol) {
            return symbol.description === "refed";
          });
          this[refed] = true;
          events.push("override");
          return this;
        }
      }
      leaked = new ImmediateNativeRefOverride(function () { events.push("callback"); });
    }, 1);
  });

  const warnings = [];
  function warningListener(warning) { warnings.push([warning.name, warning.code, warning.message]); }
  process.on("warning", warningListener);
  const errors = {
    timeoutMissing: capture(function () { setTimeout(); }),
    intervalString: capture(function () { setInterval("code", 0); }),
    immediateNumber: capture(function () { setImmediate(1); }),
    timeoutBigInt: captureMessage(function () { setTimeout(function () {}, 1n); }),
  };

  let coerced = 0;
  const delay = {
    valueOf: function () {
      coerced += 1;
      return -5;
    },
  };
  const canceledCalls = [];
  const canceledTimeout = setTimeout(function () { canceledCalls.push("timeout"); }, 0);
  const canceledImmediate = setImmediate(function () { canceledCalls.push("immediate"); });
  const canceledInterval = setInterval(function () { canceledCalls.push("interval"); }, 0);
  canceledTimeout.close();
  canceledImmediate[Symbol.dispose]();
  clearInterval(canceledInterval);

  const timeoutHandle = setTimeout(function neverRuns() {}, 60000);
  const immediateHandle = setImmediate(function neverRunsEither() {});
  function prototypeConstructorName(value) {
    const prototype = Object.getPrototypeOf(value);
    return prototype && prototype.constructor && prototype.constructor.name;
  }
  function symbolState(handle, description) {
    const symbol = Object.getOwnPropertySymbols(handle).find(function (candidate) {
      return candidate.description === description;
    });
    const descriptor = symbol && Object.getOwnPropertyDescriptor(handle, symbol);
    const value = descriptor && descriptor.value;
    return [
      description,
      value === null ? "null" : typeof value,
      descriptor && descriptor.writable,
      descriptor && descriptor.enumerable,
      descriptor && descriptor.configurable,
    ];
  }
  const retainedInstanceState = {
    timeout: {
      idlePrevPrototype: prototypeConstructorName(timeoutHandle._idlePrev),
      idleNextPrototype: prototypeConstructorName(timeoutHandle._idleNext),
      idleLinksSame: timeoutHandle._idlePrev === timeoutHandle._idleNext,
      idleStartType: typeof timeoutHandle._idleStart,
      callback: [timeoutHandle._onTimeout.name, timeoutHandle._onTimeout.length],
      timerArgsType: typeof timeoutHandle._timerArgs,
      repeatNull: timeoutHandle._repeat === null,
      destroyed: timeoutHandle._destroyed,
      symbols: ["refed", "kHasPrimitive", "asyncId", "triggerId", "kAsyncContextFrame"].map(function (description) {
        return symbolState(timeoutHandle, description);
      }),
    },
    immediate: {
      idlePrevNull: immediateHandle._idlePrev === null,
      idleNextNull: immediateHandle._idleNext === null,
      callback: [immediateHandle._onImmediate.name, immediateHandle._onImmediate.length],
      argvType: typeof immediateHandle._argv,
      destroyed: immediateHandle._destroyed,
      symbols: ["refed", "asyncId", "triggerId", "kAsyncContextFrame"].map(function (description) {
        return symbolState(immediateHandle, description);
      }),
    },
  };
  const handleShape = {
    timeout: ["close", "hasRef", "ref", "refresh", "unref"].map(function (name) { return typeof timeoutHandle[name]; }),
    immediate: ["hasRef", "ref", "unref"].map(function (name) { return typeof immediateHandle[name]; }),
    timeoutInitiallyRefed: timeoutHandle.hasRef(),
    immediateInitiallyRefed: immediateHandle.hasRef(),
    timeoutPrimitivePositive: Number(timeoutHandle) > 0,
    timeoutConstructor: [timeoutHandle.constructor.name, timeoutHandle.constructor.length],
    immediateConstructor: [immediateHandle.constructor.name, immediateHandle.constructor.length],
    timeoutSymbols: [Symbol.toPrimitive, Symbol.dispose].map(function (symbol) {
      const method = timeoutHandle[symbol];
      return [typeof method, method && method.name, method && method.length];
    }),
    immediateSymbols: [Symbol.dispose].map(function (symbol) {
      const method = immediateHandle[symbol];
      return [typeof method, method && method.name, method && method.length];
    }),
    timeoutIdleTimeout: timeoutHandle._idleTimeout,
  };
  timeoutHandle.unref();
  immediateHandle.unref();
  handleShape.timeoutUnrefed = timeoutHandle.hasRef();
  handleShape.immediateUnrefed = immediateHandle.hasRef();
  handleShape.timeoutRefIdentity = timeoutHandle.ref() === timeoutHandle;
  handleShape.timeoutRefreshIdentity = timeoutHandle.refresh() === timeoutHandle;
  handleShape.immediateRefIdentity = immediateHandle.ref() === immediateHandle;
  clearTimeout(timeoutHandle);
  clearImmediate(immediateHandle);
  handleShape.timeoutClosedRefed = timeoutHandle.hasRef();
  handleShape.immediateClosedRefed = immediateHandle.hasRef();
  handleShape.timeoutClosedIdleTimeout = timeoutHandle._idleTimeout;

  const proxyEvents = [];
  const rawProxyTimeout = setTimeout(function () { proxyEvents.push("timeout-ran"); }, 0);
  const proxyTimeout = new Proxy(new Proxy(rawProxyTimeout, {}), {});
  proxyEvents.push("timeout-unref:" + (proxyTimeout.unref() === proxyTimeout) + ":" + proxyTimeout.hasRef());
  proxyEvents.push("timeout-ref:" + (proxyTimeout.ref() === proxyTimeout) + ":" + proxyTimeout.hasRef());
  proxyEvents.push("timeout-refresh:" + (proxyTimeout.refresh() === proxyTimeout));
  clearTimeout(rawProxyTimeout);
  const rawProxyImmediate = setImmediate(function () { proxyEvents.push("immediate-ran"); });
  const proxyImmediate = new Proxy(new Proxy(rawProxyImmediate, {}), {});
  proxyEvents.push("immediate-unref:" + (proxyImmediate.unref() === proxyImmediate) + ":" + proxyImmediate.hasRef());
  proxyEvents.push("immediate-ref:" + (proxyImmediate.ref() === proxyImmediate) + ":" + proxyImmediate.hasRef());
  clearImmediate(rawProxyImmediate);

  const callbackThis = [];
  let coercedIdleTimeout;
  let refreshedIdleTimeout;
  const results = await Promise.all([
    new Promise(function (resolve) {
      const handle = setTimeout(function (left, right) {
        callbackThis.push(this && typeof this.hasRef === "function");
        resolve([left, right]);
      }, delay, "timeout", 2);
      coercedIdleTimeout = handle._idleTimeout;
    }),
    new Promise(function (resolve) {
      setImmediate(function (left, right) {
        callbackThis.push(this && typeof this.hasRef === "function");
        resolve([left, right]);
      }, "immediate", 3);
    }),
    new Promise(function (resolve) {
      let count = 0;
      const interval = setInterval(function (value) {
        count += value;
        if (count === 2) {
          clearInterval(interval);
          resolve(count);
        }
      }, 0, 1);
    }),
    new Promise(function (resolve) {
      let fired = 0;
      const refreshed = setTimeout(function () { fired += 1; resolve(fired); }, 5);
      refreshed.refresh();
      refreshedIdleTimeout = refreshed._idleTimeout;
    }),
  ]);
  await new Promise(function (resolve) { setTimeout(resolve, 5); });

  function keyToken(key) {
    return typeof key === "symbol" ? "@@" + key.description : String(key);
  }
  function valueToken(value) {
    if (value === null) return "null";
    if (value === undefined) return "undefined";
    if (typeof value === "object" || typeof value === "function") return "<object>";
    return String(value);
  }
  function traced(target, run) {
    const log = [];
    const receiver = new Proxy(target, {
      get: function (target, key, valueReceiver) {
        log.push("get " + keyToken(key));
        return Reflect.get(target, key, valueReceiver);
      },
      set: function (target, key, value, valueReceiver) {
        log.push("set " + keyToken(key) + "=" + valueToken(value));
        return Reflect.set(target, key, value, valueReceiver);
      },
    });
    const value = run(receiver);
    return { value: value === receiver ? "<receiver>" : valueToken(value), log: log };
  }

  const timeoutPrototype = Object.getPrototypeOf(timeoutHandle);
  const immediatePrototype = Object.getPrototypeOf(immediateHandle);
  const refedSymbol = Object.getOwnPropertySymbols(timeoutHandle).find(function (symbol) {
    return symbol.description === "refed";
  });
  const genericMethods = {
    timeoutHasRef: traced({}, function (receiver) {
      return timeoutPrototype.hasRef.call(receiver);
    }),
    timeoutRefUnref: traced({}, function (receiver) {
      const refIdentity = timeoutPrototype.ref.call(receiver) === receiver;
      const unrefIdentity = timeoutPrototype.unref.call(receiver) === receiver;
      return refIdentity && unrefIdentity;
    }),
    timeoutRefresh: traced({}, function (receiver) {
      return timeoutPrototype.refresh.call(receiver);
    }),
    timeoutClose: traced({ _onTimeout: function () {}, _destroyed: true }, function (receiver) {
      return timeoutPrototype.close.call(receiver);
    }),
    timeoutDispose: traced({ _onTimeout: function () {}, _destroyed: true }, function (receiver) {
      return timeoutPrototype[Symbol.dispose].call(receiver);
    }),
    timeoutPrimitive: traced({}, function (receiver) {
      return [
        valueToken(timeoutPrototype[Symbol.toPrimitive].call(receiver)),
        valueToken(timeoutPrototype[Symbol.toPrimitive].call(receiver)),
      ].join(",");
    }),
    immediateRefUnref: traced({ [refedSymbol]: false }, function (receiver) {
      const refIdentity = immediatePrototype.ref.call(receiver) === receiver;
      const unrefIdentity = immediatePrototype.unref.call(receiver) === receiver;
      return refIdentity && unrefIdentity && immediatePrototype.hasRef.call(receiver) === false;
    }),
    immediateDispose: traced({ _onImmediate: function () {}, _destroyed: true }, function (receiver) {
      return immediatePrototype[Symbol.dispose].call(receiver);
    }),
  };

  const gatedTimeout = setTimeout(function () {}, 60000);
  clearTimeout(new Proxy(gatedTimeout, {
    get: function (target, key, receiver) {
      if (key === "_onTimeout") return null;
      return Reflect.get(target, key, receiver);
    },
  }));
  const timeoutClearGated = typeof gatedTimeout._onTimeout === "function" &&
    gatedTimeout._destroyed === false && gatedTimeout._idleTimeout === 60000;
  clearTimeout(gatedTimeout);
  const gatedImmediate = setImmediate(function () {});
  clearImmediate(new Proxy(gatedImmediate, {
    get: function (target, key, receiver) {
      if (key === "_onImmediate") return null;
      return Reflect.get(target, key, receiver);
    },
  }));
  const immediateClearGated = typeof gatedImmediate._onImmediate === "function" &&
    gatedImmediate._destroyed === false && gatedImmediate.hasRef() === true;
  clearImmediate(gatedImmediate);

  const rawOmittedRef = new timeoutHandle.constructor(function () {}, 1);
  const rawOmittedRefBefore = rawOmittedRef.hasRef();
  clearTimeout(rawOmittedRef);
  const rawOmittedRefAfter = rawOmittedRef.hasRef();

  const numericCloseEvents = [];
  const numericClose = setTimeout(function () { numericCloseEvents.push("close"); }, 1);
  const numericCloseValue = Number(numericClose);
  const numericCloseReturn = timeoutPrototype.close.call(numericCloseValue);
  const numericDispose = setTimeout(function () { numericCloseEvents.push("dispose"); }, 1);
  const numericDisposeValue = Number(numericDispose);
  timeoutPrototype[Symbol.dispose].call(numericDisposeValue);

  const genericRefresh = await new Promise(function (resolve) {
    const receiver = {
      _idleTimeout: 1,
      _onTimeout: function () {
        receiver.called = this === receiver;
      },
      _repeat: null,
    };
    timeoutPrototype.refresh.call(receiver);
    setTimeout(function () {
      resolve([receiver.called === true, receiver._destroyed, receiver.hasOwnProperty("_idleStart")]);
    }, 10);
  });

  const refreshedTwice = await new Promise(function (resolve) {
    let count = 0;
    let secondDestroyed;
    const handle = setTimeout(function () {
      count += 1;
      if (count === 1) {
        this.refresh();
        return;
      }
      secondDestroyed = this._destroyed;
      clearTimeout(this);
      resolve([count, secondDestroyed, this._idleTimeout]);
    }, 1);
  });

  const liveImmediate = await new Promise(function (resolve) {
    const events = [];
    const handle = setImmediate(function () { events.push("old"); });
    handle._onImmediate = function () {
      events.push(this === handle ? "new-this" : "new-wrong-this");
      queueMicrotask(function () {
        resolve([events, handle._destroyed, handle.hasRef(), handle._onImmediate]);
      });
    };
  });

  const immediateArgumentMatrix = await new Promise(function (resolve) {
    const events = [];
    function monitor(error, origin) { events.push(["monitor", error.message, origin]); }
    function uncaught(error, origin) { events.push(["uncaught", error.message, origin]); }
    process.on("uncaughtExceptionMonitor", monitor);
    process.on("uncaughtException", uncaught);
    new immediateHandle.constructor(function () {
      events.push(["string", this instanceof immediateHandle.constructor, Array.from(arguments).join("")]);
    }, "xy");
    new immediateHandle.constructor(function () {
      events.push(["iterable", Array.from(arguments).join("")]);
    }, { [Symbol.iterator]: function* () { yield "a"; yield "b"; } });
    new immediateHandle.constructor(function () { events.push(["number-callback"]); }, 1);
    setImmediate(function () {
      process.off("uncaughtExceptionMonitor", monitor);
      process.off("uncaughtException", uncaught);
      resolve(events);
    });
  });

  const timerErrorCleanup = await new Promise(function (resolve) {
    const events = [];
    let handle;
    function record(label, error, origin) {
      events.push([label, error.message, origin, handle._destroyed, handle._idlePrev, handle._idleNext]);
    }
    function monitor(error, origin) { record("monitor", error, origin); }
    function uncaught(error, origin) { record("uncaught", error, origin); }
    process.on("uncaughtExceptionMonitor", monitor);
    process.on("uncaughtException", uncaught);
    handle = setTimeout(function () { throw new Error("timer-boom"); }, 1);
    setTimeout(function () {
      process.off("uncaughtExceptionMonitor", monitor);
      process.off("uncaughtException", uncaught);
      resolve(events);
    }, 10);
  });

  const liveRepeat = await Promise.all([
    new Promise(function (resolve) {
      let count = 0;
      const handle = setTimeout(function () {
        count += 1;
        if (count === 1) {
          this._repeat = 1;
          return;
        }
        clearTimeout(this);
        resolve(count);
      }, 1);
    }),
    new Promise(function (resolve) {
      let count = 0;
      const handle = setInterval(function () {
        count += 1;
        this._idleTimeout = -1;
      }, 1);
      setTimeout(function () {
        resolve([count, handle._destroyed]);
      }, 10);
    }),
  ]);

  const fractionalFirst = setTimeout(function () {}, 43123.75);
  const fractionalSecond = setTimeout(function () {}, 43123.25);
  const fractionalGrouped = fractionalFirst._idleNext === fractionalSecond._idlePrev;
  clearTimeout(fractionalFirst);
  clearTimeout(fractionalSecond);

  const refedFirst = setTimeout(function () {}, 43201.75);
  const refedSentinel = refedFirst._idlePrev;
  clearTimeout(refedFirst);
  const refedSecond = setTimeout(function () {}, 43201.25);
  const refedSentinelReused = refedSecond._idlePrev === refedSentinel;
  clearTimeout(refedSecond);

  const unrefedFirst = setTimeout(function () {}, 43202.75);
  const unrefedSentinel = unrefedFirst._idlePrev;
  unrefedFirst.unref();
  clearTimeout(unrefedFirst);
  const unrefedSecond = setTimeout(function () {}, 43202.25);
  const unrefedSentinelReused = unrefedSecond._idlePrev === unrefedSentinel;
  clearTimeout(unrefedSecond);

  const crossTarget = setTimeout(function () {}, 43203.75);
  const crossTargetSentinel = crossTarget._idlePrev;
  crossTarget.unref();
  clearTimeout(crossTarget);
  const crossOriginal = setTimeout(function () {}, 43204.75);
  const crossOriginalSentinel = crossOriginal._idlePrev;
  crossOriginal._idleTimeout = 43203.25;
  clearTimeout(crossOriginal);
  const crossTargetReplacement = setTimeout(function () {}, 43203.5);
  const crossOriginalReplacement = setTimeout(function () {}, 43204.5);
  const crossKeyReuse = [
    crossTargetReplacement._idlePrev === crossTargetSentinel,
    crossOriginalReplacement._idlePrev === crossOriginalSentinel,
  ];
  clearTimeout(crossTargetReplacement);
  clearTimeout(crossOriginalReplacement);

  const naturalExpiryDeletesSentinel = await new Promise(function (resolve) {
    let expiredSentinel;
    const handle = setTimeout(function () {
      setImmediate(function () {
        const replacement = setTimeout(function () {}, 3);
        const deleted = replacement._idlePrev !== expiredSentinel;
        clearTimeout(replacement);
        resolve(deleted);
      });
    }, 3);
    expiredSentinel = handle._idlePrev;
  });

  const sameDurationOrder = await new Promise(function (resolve) {
    const events = [];
    let count = 0;
    const interval = setInterval(function () {
      count += 1;
      events.push("interval-" + count);
      if (count === 1) {
        const until = performance.now() + 5;
        while (performance.now() < until) {}
        return;
      }
      clearInterval(interval);
      setImmediate(function () { resolve(events); });
    }, 20);
    setTimeout(function () { events.push("peer"); }, 20);
  });

  const refreshSemantics = await Promise.all([
    new Promise(function (resolve) {
      let reads = 0;
      const handle = setTimeout(function () {
        resolve(["single-read", reads, handle._idleTimeout]);
      }, 50);
      Object.defineProperty(handle, "_idleTimeout", {
        configurable: true,
        get: function () {
          reads += 1;
          return 1;
        },
      });
      handle.refresh();
    }),
    new Promise(function (resolve) {
      const handle = setTimeout(function () {
        resolve(["zero", handle._idleTimeout]);
      }, 50);
      handle._idleTimeout = 0;
      handle.refresh();
    }),
    new Promise(function (resolve) {
      const handle = setTimeout(function () {
        resolve(["nan", String(handle._idleTimeout)]);
      }, 5);
      handle._idleTimeout = NaN;
      handle.refresh();
    }),
    new Promise(function (resolve) {
      const handle = setTimeout(function () {}, 60000);
      handle._idleTimeout = 2147483648;
      handle.refresh();
      const sentinel = handle._idlePrev;
      const captured = [
        "overflow",
        handle._idleTimeout,
        sentinel.msecs,
        sentinel.expiry > handle._idleStart,
      ];
      clearTimeout(handle);
      resolve(captured);
    }),
  ]);

  async function destroyedRefreshPrimitiveCase(clearByPrimitive) {
    let runs = 0;
    const handle = setTimeout(function () { runs += 1; }, 1);
    const oldId = +handle;
    await new Promise(function (resolve) { setTimeout(resolve, 10); });
    handle.refresh();
    const newId = +handle;
    clearTimeout(clearByPrimitive ? newId : handle);
    await new Promise(function (resolve) { setTimeout(resolve, 10); });
    return [oldId !== newId, runs];
  }
  const destroyedRefreshPrimitive = {
    numericClear: await destroyedRefreshPrimitiveCase(true),
    objectClear: await destroyedRefreshPrimitiveCase(false),
  };

  const callbackAsyncIdSnapshot = await new Promise(function (resolve) {
    let oldId;
    const handle = setTimeout(function () {
      const asyncId = Object.getOwnPropertySymbols(this).find(function (symbol) {
        return symbol.description === "asyncId";
      });
      this[asyncId] = 9007199254740990;
      setImmediate(function () {
        clearTimeout(oldId);
        resolve([typeof handle._onTimeout, handle._destroyed]);
      });
    }, 1);
    oldId = +handle;
  });

  const callbackAsyncIdNoReread = await new Promise(function (resolve) {
    const events = [];
    setTimeout(function () {
      events.push("timer");
      const asyncId = Object.getOwnPropertySymbols(this).find(function (symbol) {
        return symbol.description === "asyncId";
      });
      Object.defineProperty(this, asyncId, {
        configurable: true,
        get: function () {
          events.push("get");
          throw new Error("async ID reread");
        },
      });
    }, 1);
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 10);
  });

  const skippedTimerCheckpoint = await new Promise(function (resolve) {
    const events = [];
    const skipped = setTimeout(function () {}, 1);
    Object.defineProperty(skipped, "_onTimeout", {
      configurable: true,
      get: function () {
        events.push("get");
        process.nextTick(function () { events.push("n"); });
        Promise.resolve().then(function () { events.push("p"); });
        return null;
      },
    });
    setTimeout(function () {
      events.push("peer");
      resolve(events);
    }, 1);
  });

  const mutableCollectionPrototypes = await new Promise(function (resolve) {
    const mapGet = Map.prototype.get;
    const mapSet = Map.prototype.set;
    const mapDelete = Map.prototype.delete;
    const arrayPush = Array.prototype.push;
    const arrayPop = Array.prototype.pop;
    try {
      Map.prototype.get = null;
      Map.prototype.set = null;
      Map.prototype.delete = null;
      Array.prototype.push = null;
      Array.prototype.pop = null;
      setTimeout(function () { resolve(true); }, 1);
    } finally {
      Map.prototype.get = mapGet;
      Map.prototype.set = mapSet;
      Map.prototype.delete = mapDelete;
      Array.prototype.push = arrayPush;
      Array.prototype.pop = arrayPop;
    }
  });

  const priorityQueueReplacementWrites = (function () {
    const root = setTimeout(function () {}, 50);
    const middle = setTimeout(function () {}, 60);
    const bottom = setTimeout(function () {}, 70);
    const list = bottom._idlePrev;
    let position = list.priorityQueuePosition;
    const writes = [];
    Object.defineProperty(list, "priorityQueuePosition", {
      configurable: true,
      get: function () { return position; },
      set: function (value) { writes.push(value); position = value; },
    });
    clearTimeout(root);
    const result = [writes, position];
    clearTimeout(middle);
    clearTimeout(bottom);
    return result;
  })();

  const priorityQueueInsertWrites = (function () {
    const seed = setTimeout(function () {}, 101);
    const prototype = Object.getPrototypeOf(seed._idlePrev);
    clearTimeout(seed);
    const writes = [];
    Object.defineProperty(prototype, "priorityQueuePosition", {
      configurable: true,
      get: function () { return this.__position; },
      set: function (value) { writes.push(value); this.__position = value; },
    });
    const handle = setTimeout(function () {}, 102);
    clearTimeout(handle);
    delete prototype.priorityQueuePosition;
    return writes;
  })();

  const corruptedQueuePosition = await new Promise(function (resolve) {
    const events = [];
    setTimeout(function () { events.push("first"); }, 1);
    const corrupted = setTimeout(function () { events.push("corrupted"); }, 10);
    corrupted._idlePrev.priorityQueuePosition = 999;
    clearTimeout(corrupted);
    setTimeout(function () {
      events.push("last");
      resolve(events);
    }, 20);
  });

  const cachedTimerListDuration = await new Promise(function (resolve) {
    const events = [];
    const first = setTimeout(function () { events.push("first"); }, 1);
    const list = first._idlePrev;
    let reads = 0;
    Object.defineProperty(list, "msecs", {
      configurable: true,
      get: function () {
        reads++;
        return reads % 2 === 1 ? 1 : 2;
      },
    });
    setTimeout(function () {
      events.push("last", "reads=" + reads);
      resolve(events);
    }, 20);
  });

  const observableUnenrollDuration = await new Promise(function (resolve) {
    const events = [];
    const stale = setTimeout(function () { events.push("stale"); }, 50);
    stale._idlePrev.msecs = 60;
    clearTimeout(stale);
    setTimeout(function () { events.push("replacement"); }, 50);
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 80);
  });

  const undefinedTimerListPeek = await new Promise(function (resolve) {
    const events = [];
    const broken = setTimeout(function () { events.push("broken"); }, 1);
    broken._idlePrev._idlePrev = undefined;
    setTimeout(function () {
      events.push("last");
      resolve(events);
    }, 20);
  });

  const mutableRootPosition = await new Promise(function (resolve) {
    const events = [];
    const first = setTimeout(function () { events.push("first"); }, 1);
    first._idlePrev.priorityQueuePosition = 999;
    setTimeout(function () {
      events.push("second");
      resolve(events);
    }, 10);
  });

  class TimeoutSubclass extends timeoutHandle.constructor {}
  class ImmediateSubclass extends immediateHandle.constructor {}
  const timeoutSubclass = new TimeoutSubclass(function () {}, 1000, undefined, false, true);
  const immediateSubclass = new ImmediateSubclass(function () {});
  const subclassConstruction = [
    timeoutSubclass instanceof TimeoutSubclass,
    Object.getPrototypeOf(timeoutSubclass) === TimeoutSubclass.prototype,
    timeoutSubclass.constructor === TimeoutSubclass,
    immediateSubclass instanceof ImmediateSubclass,
    Object.getPrototypeOf(immediateSubclass) === ImmediateSubclass.prototype,
    immediateSubclass.constructor === ImmediateSubclass,
  ];
  clearTimeout(timeoutSubclass);
  clearImmediate(immediateSubclass);

  const immediateSubclassRefOverride = await new Promise(function (resolve) {
    const events = [];
    class ImmediateRefOverride extends immediateHandle.constructor {
      ref() {
        events.push("override-ref");
        return this;
      }
    }
    const handle = new ImmediateRefOverride(function () { events.push("callback"); });
    events.push(String(handle.hasRef()));
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 10);
  });

  const immediateCapturedString = await new Promise(function (resolve) {
    const events = [];
    const originalString = String;
    const seed = setImmediate(function () {});
    const Immediate = seed.constructor;
    clearImmediate(seed);
    const args = {
      [Symbol.iterator]: function () {
        return { next: function () { return 1; } };
      },
    };
    function uncaught(error) {
      process.off("uncaughtException", uncaught);
      events.push(error.message);
    }
    process.on("uncaughtException", uncaught);
    new Immediate(function () { events.push("callback"); }, args);
    globalThis.String = function () {
      events.push("String");
      return "tampered";
    };
    setImmediate(function () {
      globalThis.String = originalString;
      events.push("peer");
      resolve(events);
    });
  });

  const immediateBatchLinks = await new Promise(function (resolve) {
    let first;
    let second;
    function links() {
      return [
        first._idlePrev === null,
        first._idleNext === second,
        second._idlePrev === first,
        second._idleNext === null,
      ];
    }
    first = setImmediate(function () {});
    second = setImmediate(function () { resolve(links()); });
  });

  const immediateVisibleLinkTraversal = await new Promise(function (resolve) {
    const events = [];
    const first = setImmediate(function () {
      events.push("first");
      first._idleNext = null;
    });
    setImmediate(function () { events.push("second"); }).unref();
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 10);
  });

  const immediateArgumentGetterThrow = await new Promise(function (resolve) {
    const events = [];
    function uncaught(error) {
      process.off("uncaughtException", uncaught);
      events.push("u:" + error.message);
    }
    process.on("uncaughtException", uncaught);
    const first = setImmediate(function () { events.push("first"); });
    Object.defineProperty(first, "_argv", {
      configurable: true,
      get: function () { throw new Error("argv"); },
    });
    setImmediate(function () { events.push("peer"); });
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 10);
  });

  const callbackOwnCall = await new Promise(function (resolve) {
    function callback() { resolve(true); }
    callback.call = null;
    setTimeout(callback, 1);
  });

  const handledThrowListGeneration = await new Promise(function (resolve) {
    const events = [];
    let sentinel;
    function uncaught() {
      process.off("uncaughtException", uncaught);
      const replacement = setTimeout(function () { events.push("replacement"); }, 1);
      events.push(replacement._idlePrev === sentinel ? "same" : "different");
    }
    process.on("uncaughtException", uncaught);
    const throwing = setTimeout(function () { throw new Error("generation"); }, 1);
    sentinel = throwing._idlePrev;
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 10);
  });

  const handledThrowFixedNow = await new Promise(function (resolve) {
    const events = [];
    function uncaught() {
      process.off("uncaughtException", uncaught);
      events.push("u");
      setImmediate(function () { events.push("i"); });
    }
    process.on("uncaughtException", uncaught);
    setTimeout(function () {
      events.push("t1");
      const until = performance.now() + 35;
      while (performance.now() < until) {}
      throw new Error("slow");
    }, 5);
    setTimeout(function () { events.push("t2"); }, 20);
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 80);
  });

  const postListCheckpoint = await new Promise(function (resolve) {
    const events = [];
    let sentinel;
    const first = setTimeout(function () {
      process.nextTick(function () {
        const replacement = setTimeout(function () { events.push("replacement"); }, 1);
        events.push(replacement._idlePrev === sentinel ? "same" : "different");
      });
    }, 1);
    sentinel = first._idlePrev;
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 20);
  });

  const nextPeerDiffCheckpoint = await new Promise(function (resolve) {
    const events = [];
    let second;
    setImmediate(function () {
      setTimeout(function () {
        process.nextTick(function () {
          second.refresh();
          setImmediate(function () { events.push("i"); });
        });
      }, 1);
      second = setTimeout(function () { events.push("second"); }, 1);
      setTimeout(function () {
        events.push("keep");
        resolve(events);
      }, 20);
    });
  });

  const boundaryCarrier = await new Promise(function (resolve) {
    setTimeout(function () {
      process.nextTick(function () {
        setTimeout(function () { resolve("later"); }, 5);
      });
    }, 1);
  });

  const handledThrowBoundaryCheckpoint = await new Promise(function (resolve) {
    const events = [];
    function uncaught() {
      process.off("uncaughtException", uncaught);
      events.push("u");
      setImmediate(function () { events.push("i"); });
    }
    process.on("uncaughtException", uncaught);
    setTimeout(function () {
      events.push("t1");
      process.nextTick(function () { events.push("n"); });
      Promise.resolve().then(function () { events.push("p"); });
      throw new Error("boundary");
    }, 1);
    setTimeout(function () {
      events.push("keep");
      resolve(events);
    }, 20);
  });

  const handledThrowSkippedCheckpoint = await new Promise(function (resolve) {
    const events = [];
    function uncaught() {
      process.off("uncaughtException", uncaught);
      events.push("u");
    }
    process.on("uncaughtException", uncaught);
    setImmediate(function () {
      events.push("i");
      setTimeout(function () {
        events.push("t1");
        process.nextTick(function () { events.push("n"); });
        Promise.resolve().then(function () { events.push("p"); });
        throw new Error("skipped checkpoint");
      }, 0);
      const skipped = setTimeout(function () { events.push("bad"); }, 0);
      skipped._onTimeout = null;
      setTimeout(function () {
        events.push("peer");
        resolve(events);
      }, 0);
    });
  });

  const handledTickThrowSkippedCheckpoint = await new Promise(function (resolve) {
    const events = [];
    function uncaught() {
      process.off("uncaughtException", uncaught);
      events.push("u");
    }
    process.on("uncaughtException", uncaught);
    setImmediate(function () {
      events.push("i");
      setTimeout(function () {
        events.push("t1");
        process.nextTick(function () {
          events.push("n1");
          throw new Error("handled tick");
        });
        process.nextTick(function () { events.push("n2"); });
        Promise.resolve().then(function () { events.push("p"); });
      }, 0);
      const skipped = setTimeout(function () { events.push("bad"); }, 0);
      skipped._onTimeout = null;
      setTimeout(function () {
        events.push("peer");
        resolve(events);
      }, 0);
    });
  });

  const priorYieldTimerSelection = await new Promise(function (resolve) {
    const events = [];
    function uncaught() {
      process.off("uncaughtException", uncaught);
      events.push("u");
    }
    process.on("uncaughtException", uncaught);
    setTimeout(function () {
      events.push("t0");
      process.nextTick(function () {
        events.push("n1");
        throw new Error("prior yield");
      });
      process.nextTick(function () { events.push("n2"); });
      Promise.resolve().then(function () { events.push("p"); });
    }, 1);
    const skipped = setTimeout(function () { events.push("bad"); }, 20);
    skipped._onTimeout = null;
    setTimeout(function () {
      events.push("peer");
      resolve(events);
    }, 20);
  });

  const destroyedImmediateRefLiveness = await new Promise(function (resolve) {
    const events = [];
    setImmediate(function () { events.push("pending"); }).unref();
    const canceled = setImmediate(function () { events.push("canceled"); });
    const refed = Object.getOwnPropertySymbols(canceled).find(function (symbol) {
      return symbol.description === "refed";
    });
    clearImmediate(canceled);
    canceled[refed] = false;
    canceled.ref();
    setTimeout(function () {
      events.push("release");
      canceled.unref();
      resolve(events);
    }, 10).unref();
  });

  const timerListShapeHandle = setTimeout(function () {}, 43301);
  const timerListShapeSentinel = timerListShapeHandle._idlePrev;
  const timerListShape = [
    timerListShapeSentinel._idleNext === timerListShapeHandle,
    timerListShapeSentinel._idlePrev === timerListShapeHandle,
    ["expiry", "id", "msecs", "priorityQueuePosition"].map(function (property) {
      return [
        property,
        Object.prototype.hasOwnProperty.call(timerListShapeSentinel, property),
        typeof timerListShapeSentinel[property],
      ];
    }),
  ];
  const clearedTimerListPosition = timerListShapeSentinel.priorityQueuePosition;
  clearTimeout(timerListShapeHandle);
  const clearedTimerListPositionRetained =
    timerListShapeSentinel.priorityQueuePosition === clearedTimerListPosition;

  const expiredTimerListPositionRetained = await new Promise(function (resolve) {
    let sentinel;
    let position;
    const handle = setTimeout(function () {
      setImmediate(function () {
        resolve(sentinel.priorityQueuePosition === position);
      });
    }, 17);
    sentinel = handle._idlePrev;
    position = sentinel.priorityQueuePosition;
  });

  const generationIdentity = await new Promise(function (resolve) {
    setTimeout(function () {
      queueMicrotask(function () {
        const tombstone = setTimeout(function () {}, 3).unref();
        const sentinel = tombstone._idleNext;
        clearTimeout(tombstone);
        setImmediate(function () {
          const replacement = setTimeout(function () {}, 3);
          const reused = replacement._idleNext === sentinel;
          clearTimeout(replacement);
          resolve(reused);
        });
      });
    }, 3);
  });

  const latentSeed = setTimeout(function () {}, 60000);
  const LatentTimeout = latentSeed.constructor;
  clearTimeout(latentSeed);
  const loneLatent = new LatentTimeout(function () {}, 1000, undefined, false, true);
  const loneLatentBefore = [
    loneLatent._destroyed,
    loneLatent.hasRef(),
    loneLatent._idlePrev === loneLatent,
    loneLatent._idleNext === loneLatent,
  ];
  loneLatent.unref();
  const loneLatentAfterUnref = [loneLatent._destroyed, loneLatent.hasRef()];
  clearTimeout(loneLatent);
  const refreshedLatent = new LatentTimeout(function () {}, 1000, undefined, false, true);
  const refreshedLatentSelfLinked = refreshedLatent._idlePrev === refreshedLatent &&
    refreshedLatent._idleNext === refreshedLatent;
  refreshedLatent.refresh();
  const refreshedLatentActive = refreshedLatent._idlePrev !== refreshedLatent &&
    refreshedLatent._idleNext !== refreshedLatent && refreshedLatent.hasRef();
  clearTimeout(refreshedLatent);
  const clearedLatent = new LatentTimeout(function () {}, 1000, undefined, false, true);
  clearTimeout(clearedLatent);
  const clearedLatentState = [
    clearedLatent._destroyed,
    clearedLatent.hasRef(),
    clearedLatent._idleTimeout,
    clearedLatent._idlePrev,
    clearedLatent._idleNext,
  ];

  await new Promise(function (resolve) { setTimeout(resolve, 5); });
  process.removeListener("warning", warningListener);
  return {
    errors: errors,
    coerced: coerced,
    canceledCalls: canceledCalls,
    callbackThis: callbackThis,
    coercedIdleTimeout: coercedIdleTimeout,
    refreshedIdleTimeout: refreshedIdleTimeout,
    retainedInstanceState: retainedInstanceState,
    handleShape: handleShape,
    proxyEvents: proxyEvents,
    genericMethods: genericMethods,
    clearGating: [timeoutClearGated, immediateClearGated],
    rawOmittedRef: [rawOmittedRefBefore, rawOmittedRefAfter],
    numericClose: [numericCloseEvents, numericCloseReturn === numericCloseValue],
    genericRefresh: genericRefresh,
    refreshedTwice: refreshedTwice,
    liveImmediate: liveImmediate,
    immediateArgumentMatrix: immediateArgumentMatrix,
    timerErrorCleanup: timerErrorCleanup,
    liveRepeat: liveRepeat,
    timerListLifecycle: {
      fractionalGrouped: fractionalGrouped,
      refedSentinelReused: refedSentinelReused,
      unrefedSentinelReused: unrefedSentinelReused,
      crossKeyReuse: crossKeyReuse,
      naturalExpiryDeletesSentinel: naturalExpiryDeletesSentinel,
    },
    sameDurationOrder: sameDurationOrder,
    refreshSemantics: refreshSemantics,
    destroyedRefreshPrimitive: destroyedRefreshPrimitive,
    callbackAsyncIdSnapshot: callbackAsyncIdSnapshot,
    callbackAsyncIdNoReread: callbackAsyncIdNoReread,
    skippedTimerCheckpoint: skippedTimerCheckpoint,
    mutableCollectionPrototypes: mutableCollectionPrototypes,
    priorityQueueReplacementWrites: priorityQueueReplacementWrites,
    priorityQueueInsertWrites: priorityQueueInsertWrites,
    corruptedQueuePosition: corruptedQueuePosition,
    cachedTimerListDuration: cachedTimerListDuration,
    observableUnenrollDuration: observableUnenrollDuration,
    undefinedTimerListPeek: undefinedTimerListPeek,
    mutableRootPosition: mutableRootPosition,
    subclassConstruction: subclassConstruction,
    immediateSubclassRefOverride: immediateSubclassRefOverride,
    immediateSubclassNativeLiveness: immediateSubclassNativeLiveness,
    immediateCapturedString: immediateCapturedString,
    immediateBatchLinks: immediateBatchLinks,
    immediateVisibleLinkTraversal: immediateVisibleLinkTraversal,
    immediateArgumentGetterThrow: immediateArgumentGetterThrow,
    callbackOwnCall: callbackOwnCall,
    handledThrowListGeneration: handledThrowListGeneration,
    handledThrowFixedNow: handledThrowFixedNow,
    postListCheckpoint: postListCheckpoint,
    nextPeerDiffCheckpoint: nextPeerDiffCheckpoint,
    boundaryCarrier: boundaryCarrier,
    handledThrowBoundaryCheckpoint: handledThrowBoundaryCheckpoint,
    handledThrowSkippedCheckpoint: handledThrowSkippedCheckpoint,
    handledTickThrowSkippedCheckpoint: handledTickThrowSkippedCheckpoint,
    priorYieldTimerSelection: priorYieldTimerSelection,
    destroyedImmediateRefLiveness: destroyedImmediateRefLiveness,
    timerListShape: timerListShape,
    retiredTimerListPosition: [clearedTimerListPositionRetained, expiredTimerListPositionRetained],
    generationIdentity: generationIdentity,
    latentTimeoutLifecycle: {
      before: loneLatentBefore,
      afterUnref: loneLatentAfterUnref,
      refresh: [refreshedLatentSelfLinked, refreshedLatentActive],
      clear: clearedLatentState,
    },
    results: results,
    warnings: warnings,
  };
}
