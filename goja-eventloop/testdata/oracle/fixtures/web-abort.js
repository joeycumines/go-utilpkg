async function webAbort() {
  function errorShape(callback) {
    try { callback(); return { ok: true }; }
    catch (error) { return { ok: false, name: error.name, code: error.code }; }
  }

  const reason = { marker: "reason" };
  const controller = new AbortController();
  const calls = [];
  controller.signal.addEventListener("abort", function () { calls.push(controller.signal.reason === reason); }, { once: true });
  controller.abort(reason);
  controller.abort("ignored");
  let thrownIdentity = false;
  try { controller.signal.throwIfAborted(); } catch (error) { thrownIdentity = error === reason; }

  const defaultSignal = AbortSignal.abort();
  const empty = AbortSignal.any([]);
  const left = new AbortController();
  const right = new AbortController();
  const combined = AbortSignal.any([left.signal, right.signal]);
  right.abort("right");

  let nextCalls = 0;
  let returnCalls = 0;
  const invalidIterable = {};
  invalidIterable[Symbol.iterator] = function () {
    return {
      next: function () {
        nextCalls += 1;
        if (nextCalls === 1) return { value: AbortSignal.abort("early"), done: false };
        if (nextCalls === 2) return { value: 1, done: false };
        return { done: true };
      },
      return: function () { returnCalls += 1; return { done: true }; },
    };
  };
  const invalidLater = errorShape(function () { AbortSignal.any(invalidIterable); });
  const invalidContainer = errorShape(function () { AbortSignal.any(1); });

  let timeoutCoercions = 0;
  const timeoutConversion = {
    accepted: [null, false, true, "", "1", 1.5, -0.5, 4294967296, {
      valueOf: function () { timeoutCoercions += 1; return 7.8; },
    }].map(function (value) {
      return errorShape(function () { AbortSignal.timeout(value); });
    }),
    rejected: [undefined, NaN, -1, Infinity, -Infinity, 9007199254740992, Symbol("delay"), 1n].map(function (value) {
      return errorShape(function () { AbortSignal.timeout(value); });
    }),
  };
  timeoutConversion.coercions = timeoutCoercions;
  const timeoutSignal = AbortSignal.timeout(0);
  await new Promise(function (resolve, reject) {
    const guard = setTimeout(function () { reject(new Error("AbortSignal.timeout did not fire")); }, 1000);
    timeoutSignal.addEventListener("abort", function () {
      clearTimeout(guard);
      resolve();
    }, { once: true });
  });

  const propertyController = new AbortController();
  const propertySignal = propertyController.signal;
  const onabortCalls = [];
  const onabortDefault = propertySignal.onabort;
  function firstOnabort() { onabortCalls.push("first"); }
  function secondOnabort(event) {
    onabortCalls.push("second:" + (this === propertySignal) + ":" + (event.target === propertySignal));
  }
  propertySignal.onabort = firstOnabort;
  const firstOnabortIdentity = propertySignal.onabort === firstOnabort;
  propertySignal.addEventListener("abort", function () { onabortCalls.push("listener"); });
  propertySignal.onabort = secondOnabort;
  const secondOnabortIdentity = propertySignal.onabort === secondOnabort;
  propertyController.abort("property");
  propertySignal.onabort = 1;
  const clearedOnabort = propertySignal.onabort;

  return {
    aborted: controller.signal.aborted,
    reasonIdentity: controller.signal.reason === reason,
    calls: calls,
    thrownIdentity: thrownIdentity,
    defaultReason: [defaultSignal.reason instanceof DOMException, defaultSignal.reason.name, defaultSignal.reason.code],
    empty: [empty.aborted, typeof empty.reason],
    combined: [combined.aborted, combined.reason],
    invalidLater: invalidLater,
    invalidContainer: invalidContainer,
    iterator: [nextCalls, returnCalls],
    timeoutConversion: timeoutConversion,
    timeout: [timeoutSignal.aborted, timeoutSignal.reason.name, timeoutSignal.reason.code],
    onabort: [
      onabortDefault === null,
      firstOnabortIdentity,
      secondOnabortIdentity,
      onabortCalls,
      clearedOnabort === null,
    ],
  };
}
