async function nodePromises() {
  function errorShape(error) { return { name: error.name, code: error.code }; }

  const capability = Promise.withResolvers();
  queueMicrotask(function () { capability.resolve("resolved"); });
  const settled = await Promise.allSettled([Promise.resolve(1), Promise.reject("x")]);
  const any = await Promise.any([Promise.reject("no"), Promise.resolve("yes")]);
  const tried = await Promise.try(function (a, b) { return a + b; }, 2, 3);
  const sequence = [];
  const value = await Promise.resolve(4).then(function (entry) {
    sequence.push("then");
    return entry + 1;
  }).finally(function () { sequence.push("finally"); });

  const errors = {};
  try { await Promise.all(null); } catch (error) { errors.allNull = errorShape(error); }
  try { await Promise.any([]); } catch (error) {
    errors.anyEmpty = { name: error.name, errors: Array.from(error.errors) };
  }
  try { Promise.withResolvers.call({}); } catch (error) { errors.withResolversReceiver = errorShape(error); }
  try { Promise.try.call({}, function () {}); } catch (error) { errors.tryReceiver = errorShape(error); }

  const abrupt = [];
  let iteratorClosed = false;
  const iterable = {};
  iterable[Symbol.iterator] = function () {
    return {
      next: function () { throw new Error("iterator boom"); },
      return: function () { iteratorClosed = true; return { done: true }; },
    };
  };
  try { await Promise.all(iterable); } catch (error) { abrupt.push(error.message); }

  const iterableErrors = [];
  const invalidInputs = [
    ["null", function () { return null; }],
    ["undefined", function () { return undefined; }],
    ["object", function () { return {}; }],
    ["number", function () { return 1; }],
    ["symbol", function () { return Symbol("probe"); }],
    ["method", function () { return { [Symbol.iterator]: 1 }; }],
    ["iterator", function () { return { [Symbol.iterator]: function () { return 1; } }; }],
    ["next", function () { return { [Symbol.iterator]: function () { return { next: 1 }; } }; }],
    ["result", function () { return { [Symbol.iterator]: function () { return { next: function () { return 1; } }; } }; }],
    ["null-result", function () { return { [Symbol.iterator]: function () { return { next: function () { return null; } }; } }; }],
  ];
  for (const method of ["all", "race", "allSettled", "any"]) {
    for (const [name, input] of invalidInputs) {
      try { await Promise[method](input()); }
      catch (error) { iterableErrors.push([method, name, error.name, error.message]); }
    }
  }

  const capabilityErrors = [];
  const NativeTypeError = TypeError;
  let poisonedTypeErrorCalls = 0;
  globalThis.TypeError = function PoisonTypeError() { poisonedTypeErrorCalls++; };
  function captureCapability(method, name, receiver) {
    try { Promise[method].call(receiver, []); }
    catch (error) {
      capabilityErrors.push([method, name, error instanceof NativeTypeError, error.name, error.message]);
    }
  }
  function Twice(executor) {
    executor(function () {}, function () {});
    executor(function () {}, function () {});
    return {};
  }
  function Missing() { return {}; }
  for (const method of ["all", "race", "allSettled", "any"]) {
    captureCapability(method, "null", null);
    captureCapability(method, "object", {});
    captureCapability(method, "null-prototype", Object.create(null));
    captureCapability(method, "twice", Twice);
    captureCapability(method, "missing", Missing);
  }
  globalThis.TypeError = NativeTypeError;

  const genericCallbacks = [];
  function isConstructor(value) {
    try { Reflect.construct(function () {}, [], value); return true; }
    catch (_) { return false; }
  }
  function callbackShape(callback) {
    return [callback.name, callback.length, Object.hasOwn(callback, "prototype"), isConstructor(callback)];
  }
  for (const method of ["all", "race", "allSettled", "any", "withResolvers", "try"]) {
    function Constructor(executor) {
      genericCallbacks.push([method + ".executor", callbackShape(executor)]);
      executor(function () {}, function () {});
      return { method: method };
    }
    Constructor.resolve = function () { return { then: function () {} }; };
    if (method === "withResolvers") Promise.withResolvers.call(Constructor);
    else if (method === "try") Promise.try.call(Constructor, function () { return 1; });
    else Promise[method].call(Constructor, []);
  }
  let genericResolve;
  let genericReject;
  let genericMethod;
  function GenericConstructor(executor) {
    genericResolve = function genericResolve(value) {
      genericCallbacks.push([genericMethod + ".resolve", value]);
      return "resolve-return";
    };
    genericReject = function genericReject(reason) {
      if (reason instanceof AggregateError) {
        genericCallbacks.push([genericMethod + ".reject", reason.name, reason.message, reason.errors[0]]);
      } else genericCallbacks.push([genericMethod + ".reject", reason]);
      return "reject-return";
    };
    executor(genericResolve, genericReject);
    return { method: genericMethod };
  }
  GenericConstructor.resolve = function (entry) {
    return {
      then: function (onFulfilled, onRejected) {
        if (genericMethod === "all") {
          genericCallbacks.push(["all.handlers", callbackShape(onFulfilled), onRejected === genericReject]);
          onFulfilled(entry);
        } else if (genericMethod === "race") {
          genericCallbacks.push(["race.handlers", onFulfilled === genericResolve, onRejected === genericReject]);
        } else if (genericMethod === "allSettled") {
          genericCallbacks.push(["allSettled.handlers", callbackShape(onFulfilled), callbackShape(onRejected)]);
          onRejected("settled-reason");
        } else {
          genericCallbacks.push(["any.handlers", onFulfilled === genericResolve, callbackShape(onRejected)]);
          onRejected("any-reason");
          genericCallbacks.push(["any.resolveReturn", onFulfilled("any-value")]);
        }
      },
    };
  };
  for (genericMethod of ["all", "race", "allSettled", "any"]) {
    Promise[genericMethod].call(GenericConstructor, [genericMethod]);
  }

  const finalResolveFailures = [];
  for (const method of ["all", "allSettled"]) {
    const events = [];
    function ThrowingResolve(executor) {
      executor(
        function (value) {
          events.push(["resolve", value.length]);
          throw new Error(method + " resolve boom");
        },
        function (reason) { events.push(["reject", reason.message]); },
      );
      return { method: method };
    }
    ThrowingResolve.resolve = function () { return { then: function () {} }; };
    try {
      const result = Promise[method].call(ThrowingResolve, []);
      events.push(["returned", result.method]);
    } catch (error) {
      events.push(["threw", error.message]);
    }
    finalResolveFailures.push([method, events]);
  }

  function onePromiseEntry(entry) {
    return {
      [Symbol.iterator]: function () {
        let pending = true;
        return {
          next: function () {
            if (!pending) return { done: true };
            pending = false;
            return { done: false, value: entry };
          },
        };
      },
    };
  }
  let prototypeSetterCalls = 0;
  let prototypeAll;
  let prototypeSettled;
  let prototypeAny;
  let prototypeTry;
  Object.defineProperty(Array.prototype, "0", {
    configurable: true,
    set: function () { prototypeSetterCalls++; },
  });
  Object.defineProperty(Object.prototype, "reason", {
    configurable: true,
    set: function () { prototypeSetterCalls++; },
  });
  Object.defineProperty(Object.prototype, "value", {
    configurable: true,
    set: function () { prototypeSetterCalls++; },
  });
  try {
    function PrototypeAll(executor) {
      executor(function (value) { prototypeAll = value; }, function (reason) { prototypeAll = reason; });
      return {};
    }
    PrototypeAll.resolve = function (entry) { return { then: function (resolve) { resolve(entry); } }; };
    Promise.all.call(PrototypeAll, onePromiseEntry(11));

    function PrototypeSettled(executor) {
      executor(function (value) { prototypeSettled = value; }, function (reason) { prototypeSettled = reason; });
      return {};
    }
    PrototypeSettled.resolve = function () {
      return { then: function (resolve, reject) { reject(12); } };
    };
    Promise.allSettled.call(PrototypeSettled, onePromiseEntry(12));

    function PrototypeAny(executor) {
      executor(function () {}, function (reason) { prototypeAny = reason; });
      return {};
    }
    PrototypeAny.resolve = function (entry) {
      return { then: function (resolve, reject) { reject(entry); } };
    };
    Promise.any.call(PrototypeAny, onePromiseEntry(13));

    function PrototypeTry(executor) {
      executor(function (value) { prototypeTry = value; }, function (reason) { prototypeTry = reason; });
      return {};
    }
    Promise.try.call(PrototypeTry, function (entry) { return entry; }, 14);
  } finally {
    delete Array.prototype[0];
    delete Object.prototype.reason;
    delete Object.prototype.value;
  }
  const prototypeMutation = {
    setterCalls: prototypeSetterCalls,
    all: prototypeAll,
    settled: prototypeSettled,
    any: [prototypeAny.name, prototypeAny.message, prototypeAny.errors],
    tried: prototypeTry,
  };

  const originalArrayIterator = Array.prototype[Symbol.iterator];
  const aggregateToken = {};
  let aggregateRejection;
  let aggregateTrace;
  function AggregateConstructor(executor) {
    executor(function () {}, function (reason) { aggregateRejection = reason; });
    return aggregateToken;
  }
  AggregateConstructor.resolve = function (entry) { return entry; };
  const aggregateEmpty = {
    [Symbol.iterator]: function () {
      return { next: function () { return { done: true }; } };
    },
  };
  Array.prototype[Symbol.iterator] = function () { throw new Error("array iterator poison"); };
  try {
    aggregateTrace = Promise.any.call(AggregateConstructor, aggregateEmpty) === aggregateToken
      ? "returned"
      : "wrong-return";
  } catch (error) {
    aggregateTrace = "threw:" + error.message;
  } finally {
    Array.prototype[Symbol.iterator] = originalArrayIterator;
  }
  const aggregateMutation = aggregateRejection === undefined
    ? [aggregateTrace, "missing-rejection"]
    : [aggregateTrace, aggregateRejection.name, aggregateRejection.message, aggregateRejection.errors.length];

  const nullishResolveErrors = [];
  for (const method of ["all", "race", "allSettled", "any"]) {
    for (const entry of [undefined, null]) {
      let rejection;
      function NullishResolve(executor) {
        executor(function () {}, function (reason) { rejection = reason; });
        return {};
      }
      NullishResolve.resolve = function () { return entry; };
      Promise[method].call(NullishResolve, [1]);
      nullishResolveErrors.push([method, String(entry), rejection.name, rejection.message]);
    }
  }

  const rejectionEvents = [];
  function onUnhandled(reason, promise) { rejectionEvents.push(["unhandled", reason, promise === rejected]); }
  function onHandled(promise) { rejectionEvents.push(["handled", promise === rejected]); }
  process.on("unhandledRejection", onUnhandled);
  process.on("rejectionHandled", onHandled);
  const rejected = Promise.reject("checkpoint");
  await new Promise(function (resolve) { setImmediate(resolve); });
  rejected.catch(function () {});
  await new Promise(function (resolve) { setImmediate(resolve); });
  process.removeListener("unhandledRejection", onUnhandled);
  process.removeListener("rejectionHandled", onHandled);

  return {
    capability: await capability.promise,
    settled: settled,
    any: any,
    tried: tried,
    value: value,
    sequence: sequence,
    errors: errors,
    abrupt: abrupt,
    iteratorClosed: iteratorClosed,
    iterableErrors: iterableErrors,
    capabilityErrors: capabilityErrors,
    poisonedTypeErrorCalls: poisonedTypeErrorCalls,
    genericCallbacks: genericCallbacks,
    finalResolveFailures: finalResolveFailures,
    prototypeMutation: prototypeMutation,
    aggregateMutation: aggregateMutation,
    nullishResolveErrors: nullishResolveErrors,
    rejectionEvents: rejectionEvents,
  };
}
