async function extensionDelay() {
  const extensionType = typeof globalThis.delay;
  if (extensionType !== "function") return { extensionType: extensionType };
  const order = [];
  let coercions = 0;
  const pending = delay({
    valueOf: function () {
      coercions += 1;
      order.push("coerce");
      return -1;
    },
  });
  const promiseBrand = pending instanceof Promise;
  pending.then(function extensionDelayFulfilled(value) {
    order.push(value === undefined ? "fulfill:undefined" : "fulfill:other");
  });
  order.push("return");
  const value = await pending;
  order.push("await");
  const missing = await delay();
  return {
    extensionType: extensionType,
    value: value,
    missing: missing,
    promiseBrand: promiseBrand,
    coercions: coercions,
    order: order,
  };
}
