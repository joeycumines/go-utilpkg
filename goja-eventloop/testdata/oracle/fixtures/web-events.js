function webEvents() {
  const target = new EventTarget();
  const calls = [];
  function listener(event) {
    calls.push([event.type, event.detail, event.target === target, event.currentTarget === target, event.eventPhase]);
  }
  target.addEventListener("oracle", listener, { once: true });
  target.addEventListener("oracle", listener);
  target.addEventListener("oracle", listener);
  const firstEvent = new CustomEvent("oracle", { detail: 7, cancelable: true, composed: true });
  const first = target.dispatchEvent(firstEvent);
  target.removeEventListener("oracle", listener);
  const second = target.dispatchEvent(new Event("oracle"));

  const passiveEvent = new Event("passive", { cancelable: true });
  target.addEventListener("passive", function (event) { event.preventDefault(); }, { passive: true });
  const passiveDispatch = target.dispatchEvent(passiveEvent);

  const activeEvent = new Event("active", { cancelable: true });
  target.addEventListener("active", function (event) { event.preventDefault(); });
  const activeDispatch = target.dispatchEvent(activeEvent);

  const controller = new AbortController();
  let signalCalls = 0;
  target.addEventListener("signal", function () { signalCalls += 1; }, { signal: controller.signal });
  controller.abort("remove");
  target.dispatchEvent(new Event("signal"));

  let objectCalls = 0;
  const objectListener = { handleEvent: function () { objectCalls += 1; } };
  target.addEventListener("object", objectListener);
  target.dispatchEvent(new Event("object"));
  target.removeEventListener("object", objectListener);
  target.dispatchEvent(new Event("object"));

  const legacy = new Event("legacy", { bubbles: true, cancelable: true, composed: true });
  const legacyInitial = [legacy.srcElement, legacy.returnValue, legacy.cancelBubble, legacy.composed, legacy.isTrusted];
  legacy.returnValue = false;
  legacy.cancelBubble = true;
  const legacyMutated = [legacy.defaultPrevented, legacy.returnValue, legacy.cancelBubble];
  legacy.initEvent("initialized", false, true);
  const legacyInitialized = [legacy.type, legacy.bubbles, legacy.cancelable, legacy.defaultPrevented];

  const initializedCustom = new CustomEvent("before", { detail: 1 });
  let customInitError = null;
  try {
    initializedCustom.initCustomEvent("after", true, true, 9);
  } catch (error) {
    customInitError = { name: error.name, code: error.code };
  }
  const customInitialized = [initializedCustom.type, initializedCustom.bubbles, initializedCustom.cancelable, initializedCustom.detail];
  const constants = ["NONE", "CAPTURING_PHASE", "AT_TARGET", "BUBBLING_PHASE"].map(function (name) {
    const constructorDescriptor = Object.getOwnPropertyDescriptor(Event, name);
    const prototypeDescriptor = Object.getOwnPropertyDescriptor(Event.prototype, name);
    return [
      Event[name], Event.prototype[name],
      constructorDescriptor.writable, constructorDescriptor.enumerable, constructorDescriptor.configurable,
      Boolean(prototypeDescriptor),
      prototypeDescriptor && prototypeDescriptor.writable,
      prototypeDescriptor && prototypeDescriptor.enumerable,
      prototypeDescriptor && prototypeDescriptor.configurable,
    ];
  });
  const trustedDescriptor = Object.getOwnPropertyDescriptor(legacy, "isTrusted");

  const propagation = [];
  const propagationTarget = new EventTarget();
  propagationTarget.addEventListener("normal", function (event) {
    propagation.push("normal-first:" + event.cancelBubble);
    event.stopPropagation();
    propagation.push("normal-stopped:" + event.cancelBubble);
  });
  propagationTarget.addEventListener("normal", function () { propagation.push("normal-second"); });
  propagationTarget.addEventListener("immediate", function (event) {
    propagation.push("immediate-first");
    event.stopImmediatePropagation();
  });
  propagationTarget.addEventListener("immediate", function () { propagation.push("immediate-second"); });
  propagationTarget.dispatchEvent(new Event("normal"));
  propagationTarget.dispatchEvent(new Event("immediate"));

  const beforeTimeStamp = performance.now();
  const timestampEvent = new Event("timestamp");
  const firstTimeStamp = timestampEvent.timeStamp;
  const afterTimeStamp = performance.now();
  const timestampTarget = new EventTarget();
  let dispatchTimeStamp = -1;
  timestampTarget.addEventListener("timestamp", function (event) { dispatchTimeStamp = event.timeStamp; });
  timestampTarget.dispatchEvent(timestampEvent);
  const timeStamp = [
    Number.isFinite(firstTimeStamp),
    firstTimeStamp >= 0,
    firstTimeStamp >= beforeTimeStamp - 1,
    firstTimeStamp <= afterTimeStamp + 1,
    dispatchTimeStamp === firstTimeStamp,
    timestampEvent.timeStamp === firstTimeStamp,
  ];

  return {
    calls: calls,
    first: first,
    second: second,
    postDispatch: [firstEvent.target === target, firstEvent.currentTarget === null, firstEvent.eventPhase],
    passive: [passiveDispatch, passiveEvent.defaultPrevented],
    active: [activeDispatch, activeEvent.defaultPrevented],
    signalCalls: signalCalls,
    objectCalls: objectCalls,
    composedPathAfter: firstEvent.composedPath(),
    legacyInitial: legacyInitial,
    legacyMutated: legacyMutated,
    legacyInitialized: legacyInitialized,
    customInitialized: customInitialized,
    customInitError: customInitError,
    constants: constants,
    isTrustedDescriptor: [Boolean(trustedDescriptor), trustedDescriptor && trustedDescriptor.enumerable, trustedDescriptor && trustedDescriptor.configurable],
    propagation: propagation,
    timeStamp: timeStamp,
  };
}
