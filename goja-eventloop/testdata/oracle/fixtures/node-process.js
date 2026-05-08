function nodeProcess() {
  const initialEmitterState = {
    count: process._eventsCount,
    keys: Reflect.ownKeys(process._events).map(function (key) { return String(key); }).sort(),
    maxListeners: String(process._maxListeners),
    listeners: ["newListener", "removeListener"].map(function (name) {
      const listener = process._events[name];
      return [name, typeof listener, listener && listener.name, listener && listener.length];
    }),
  };
  function dataState(owner, key) {
    const descriptor = Object.getOwnPropertyDescriptor(owner, key);
    const value = descriptor.value;
    return [
      value === null ? "null" : typeof value,
      value === undefined ? "undefined" : value,
      descriptor.writable,
      descriptor.enumerable,
      descriptor.configurable,
    ];
  }
  function symbolDescription(owner, description) {
    return Object.getOwnPropertySymbols(owner).find(function (symbol) {
      return symbol.description === description;
    });
  }
  const processPrototype = Object.getPrototypeOf(process);
  const emitterPrototype = Object.getPrototypeOf(processPrototype);
  const processCapture = symbolDescription(process, "kCapture");
  const emitterCapture = symbolDescription(emitterPrototype, "kCapture");
  const retainedPrototypeState = {
    constructor: [
      processPrototype.constructor.name,
      processPrototype.constructor.length,
      dataState(processPrototype, "constructor"),
    ],
    defaults: ["_events", "_eventsCount", "_maxListeners"].map(function (name) {
      return [name, dataState(emitterPrototype, name)];
    }),
    capture: [
      dataState(process, processCapture),
      dataState(emitterPrototype, emitterCapture),
      process[processCapture] === emitterPrototype[emitterCapture],
    ],
  };

  function descriptorDepth(object, property) {
    let current = object;
    let depth = 0;
    while (current !== null) {
      if (Object.prototype.hasOwnProperty.call(current, property)) return depth;
      current = Object.getPrototypeOf(current);
      depth += 1;
    }
    return -1;
  }

  function capture(label, callback, values) {
    try {
      callback();
      values.push(label + ":ok");
    } catch (error) {
      values.push(label + ":" + error.name + ":" + error.code);
    }
  }

  const calls = [];
  function retained(value) { calls.push("on:" + value); }
  function once(value) { calls.push("once:" + value); }
  process.on("oracle", retained);
  process.once("oracle", once);
  const first = process.emit("oracle", "a");
  const second = process.emit("oracle", "b");
  const beforeRemoval = process.listenerCount("oracle");
  process.removeListener("oracle", retained);
  const afterRemoval = process.listenerCount("oracle");

  function chainListener() {}
  const identities = {
    onAddListener: process.on === process.addListener,
    offRemoveListener: process.off === process.removeListener,
    onReturnsProcess: process.on("oracle-chain", chainListener) === process,
    offReturnsProcess: process.off("oracle-chain", chainListener) === process,
  };
  function directPrototypeOperations() {
    function a() {}
    function b() {}
    function c() {}

    function observe(target, key, receiver, action) {
      const original = target[key];
      let calls = 0;
      target[key] = function(...args) {
        if (receiver === undefined || this === receiver) calls++;
        return Reflect.apply(original, this, args);
      };
      try {
        action();
      } finally {
        target[key] = original;
      }
      return calls;
    }

    const pushType = Symbol("oracle-direct-push");
    process.on(pushType, a);
    process.on(pushType, b);
    const pushCalls = observe(
      Array.prototype,
      "push",
      process._events[pushType],
      function() { process.on(pushType, c); },
    );

    const popType = Symbol("oracle-direct-pop");
    process.on(popType, a);
    process.on(popType, b);
    process.on(popType, c);
    const popCalls = observe(
      Array.prototype,
      "pop",
      process._events[popType],
      function() { process.removeListener(popType, c); },
    );

    const shiftType = Symbol("oracle-direct-shift");
    process.on(shiftType, a);
    process.on(shiftType, b);
    process.on(shiftType, c);
    const shiftCalls = observe(
      Array.prototype,
      "shift",
      process._events[shiftType],
      function() { process.removeListener(shiftType, a); },
    );

    const bindType = Symbol("oracle-direct-bind");
    const bindCalls = observe(
      Function.prototype,
      "bind",
      undefined,
      function() { process.once(bindType, a); },
    );

    const callType = Symbol("oracle-direct-call");
    let listenerCalls = 0;
    function callListener() { listenerCalls++; }
    process.once(callType, callListener);
    const callCalls = observe(
      Function.prototype,
      "call",
      callListener,
      function() { process.emit(callType); },
    );

    return {
      pushCalls: pushCalls,
      popCalls: popCalls,
      shiftCalls: shiftCalls,
      bindCalls: bindCalls,
      callCalls: callCalls,
      listenerCalls: listenerCalls,
      callRemaining: process.listenerCount(callType),
    };
  }
  const directOperations = directPrototypeOperations();
  const eventMethods = [
    "on", "addListener", "once", "off", "removeListener", "emit",
    "listenerCount",
  ];
  const methodDepths = {};
  eventMethods.forEach(function (name) { methodDepths[name] = descriptorDepth(process, name); });
  const processPrototypeChain = [];
  let prototype = process;
  for (let depth = 0; prototype !== null && depth < 4; depth += 1) {
    processPrototypeChain.push(prototype.constructor && prototype.constructor.name);
    prototype = Object.getPrototypeOf(prototype);
  }
  const exitCodeDescriptor = Object.getOwnPropertyDescriptor(process, "exitCode");
  const exitCodeShape = {
    configurable: exitCodeDescriptor.configurable,
    enumerable: exitCodeDescriptor.enumerable,
    getter: [exitCodeDescriptor.get.name, exitCodeDescriptor.get.length],
    setter: [exitCodeDescriptor.set.name, exitCodeDescriptor.set.length],
  };

  const processProxy = new Proxy(new Proxy(process, {}), {});
  const proxyEvents = [];
  function proxyListener(value) {
    proxyEvents.push(value + ":" + (this === process ? "process" : this === processProxy ? "proxy" : "other"));
  }
  const proxyReturned = processProxy.on("oracle-proxy", proxyListener) === processProxy;
  const proxyCounts = [process.listenerCount("oracle-proxy"), processProxy.listenerCount("oracle-proxy")];
  const proxyProcessEmit = process.emit("oracle-proxy", "direct");
  const proxyEmit = processProxy.emit("oracle-proxy", "proxied");
  processProxy.off("oracle-proxy", proxyListener);
  const proxyAfter = process.listenerCount("oracle-proxy");

  const codes = [];
  capture("string", function () { process.exitCode = "abc"; }, codes);
  capture("fraction", function () { process.exitCode = 1.5; }, codes);
  capture("object", function () { process.exitCode = { valueOf: function () { return 5; } }; }, codes);
  process.exitCode = "0x10";
  codes.push("hex:" + process.exitCode);
  process.exitCode = 4294967295;
  codes.push("wrapped:" + process.exitCode);
  process.exitCode = null;
  codes.push("null:" + String(process.exitCode));

  const warnings = [];
  process.on("warning", function (warning) {
    warnings.push([
      warning.name,
      warning.message,
      warning.code === undefined ? "undefined" : warning.code,
      warning.detail === undefined ? "undefined" : warning.detail,
    ]);
  });
  process.emitWarning("oracle warning", {
    type: "OracleWarning",
    code: "E_ORACLE",
    detail: "oracle detail",
  });
  const warningError = new Error("error warning");
  warningError.name = "OracleError";
  warningError.code = "E_ERROR";
  warningError.detail = "error detail";
  process.emitWarning(warningError);
  const warningThrown = {
    [Symbol.toPrimitive]: function () {
      warningThrown.coercions += 1;
      throw new Error("warning exception was coerced");
    },
    coercions: 0,
  };
  const hostileWarning = new Error("hostile warning");
  Object.defineProperty(hostileWarning, "name", {
    configurable: true,
    get: function () { throw warningThrown; },
  });
  let warningThrownIdentity = false;
  try { process.emitWarning(hostileWarning); }
  catch (error) { warningThrownIdentity = error === warningThrown; }

  const originalExiting = Object.getOwnPropertyDescriptor(process, "_exiting");
  const exitThrown = {
    [Symbol.toPrimitive]: function () {
      exitThrown.coercions += 1;
      throw new Error("process.exit exception was coerced");
    },
    coercions: 0,
  };
  let exitSetterCalls = 0;
  Object.defineProperty(process, "_exiting", {
    configurable: true,
    enumerable: originalExiting.enumerable,
    get: function () { return false; },
    set: function () { exitSetterCalls += 1; throw exitThrown; },
  });
  let exitThrownIdentity = false;
  try { process.exit(17); }
  catch (error) { exitThrownIdentity = error === exitThrown; }
  const explicitExitSetter = [
    exitThrownIdentity,
    exitSetterCalls,
    exitThrown.coercions,
    process.exitCode,
    process._exiting,
  ];
  Object.defineProperty(process, "_exiting", originalExiting);
  process.exitCode = null;

  const lifecycle = [];
  let beforeExitCount = 0;
  process.on("beforeExit", function (code) {
    beforeExitCount += 1;
    lifecycle.push("beforeExit:" + code + ":" + String(process.exitCode));
    if (beforeExitCount === 1) {
      process.nextTick(function () { lifecycle.push("nextTick"); });
      Promise.resolve().then(function () { lifecycle.push("promise"); });
      setImmediate(function () { lifecycle.push("immediate"); });
    } else if (beforeExitCount === 2) {
      lifecycle.push("explicit-exit:" + code);
      process.exit("256");
      lifecycle.push("after-explicit-exit");
    }
  });
  process.on("exit", function (code) {
    lifecycle.push("exit:" + code + ":" + process._exiting + ":" + String(process.exitCode));
    process.nextTick(function () { lifecycle.push("exit-nextTick"); });
    queueMicrotask(function () { lifecycle.push("exit-microtask"); });
    Promise.resolve().then(function () { lifecycle.push("exit-promise"); });
    setImmediate(function () { lifecycle.push("exit-immediate"); });
    setTimeout(function () { lifecycle.push("exit-timeout"); }, 0);
  });
  return {
    initialEmitterState: initialEmitterState,
    retainedPrototypeState: retainedPrototypeState,
    calls: calls,
    first: first,
    second: second,
    beforeRemoval: beforeRemoval,
    afterRemoval: afterRemoval,
    codes: codes,
    warnings: warnings,
    warningThrown: [warningThrownIdentity, warningThrown.coercions],
    explicitExitSetter: explicitExitSetter,
    identities: identities,
    directPrototypeOperations: directOperations,
    methodDepths: methodDepths,
    processPrototypeChain: processPrototypeChain,
    exitCodeDescriptor: exitCodeShape,
    proxy: {
      returned: proxyReturned,
      counts: proxyCounts,
      processEmit: proxyProcessEmit,
      emit: proxyEmit,
      after: proxyAfter,
      events: proxyEvents,
    },
    lifecycle: lifecycle,
  };
}
