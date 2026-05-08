function webCrypto() {
  function errorShape(callback) {
    try { callback(); return { ok: true }; }
    catch (error) { return { ok: false, name: error.name, code: error.code, constructor: error.constructor.name }; }
  }

  const constructors = [Int8Array, Uint8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array, Uint32Array];
  if (typeof BigInt64Array === "function") constructors.push(BigInt64Array, BigUint64Array);
  const allowed = constructors.map(function (Constructor) {
    const view = new Constructor(4);
    return [Constructor.name, crypto.getRandomValues(view) === view];
  });

  const backing = new Uint8Array(16);
  backing.fill(170);
  const offset = new Uint8Array(backing.buffer, 4, 4);
  const sameOffset = crypto.getRandomValues(offset) === offset;
  const outside = Array.from(backing.slice(0, 4)).concat(Array.from(backing.slice(8)));

  const detachedBuffer = new ArrayBuffer(8);
  const detachedView = new Uint8Array(detachedBuffer);
  structuredClone(detachedBuffer, { transfer: [detachedBuffer] });

  const getRandomValues = crypto.getRandomValues;
  const randomUUID = crypto.randomUUID;
  const uuid = randomUUID.call(crypto);
  return {
    brand: [crypto instanceof Crypto, Object.getPrototypeOf(crypto) === Crypto.prototype, Object.prototype.toString.call(crypto), typeof crypto.subtle],
    allowed: allowed,
    offset: [sameOffset, outside.every(function (value) { return value === 170; })],
    uuidV4: /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(uuid),
    errors: {
      missing: errorShape(function () { getRandomValues.call(crypto); }),
      undefined: errorShape(function () { getRandomValues.call(crypto, undefined); }),
      null: errorShape(function () { getRandomValues.call(crypto, null); }),
      plain: errorShape(function () { getRandomValues.call(crypto, {}); }),
      array: errorShape(function () { getRandomValues.call(crypto, []); }),
      buffer: errorShape(function () { getRandomValues.call(crypto, new ArrayBuffer(1)); }),
      spoof: errorShape(function () { getRandomValues.call(crypto, Object.create(Uint8Array.prototype)); }),
      proxy: errorShape(function () { getRandomValues.call(crypto, new Proxy(new Uint8Array(1), {})); }),
      float32: errorShape(function () { getRandomValues.call(crypto, new Float32Array(1)); }),
      float64: errorShape(function () { getRandomValues.call(crypto, new Float64Array(1)); }),
      dataView: errorShape(function () { getRandomValues.call(crypto, new DataView(new ArrayBuffer(1))); }),
      quota: errorShape(function () { getRandomValues.call(crypto, new Uint8Array(65537)); }),
      detached: errorShape(function () { getRandomValues.call(crypto, detachedView); }),
      getReceiver: errorShape(function () { getRandomValues.call({}, new Uint8Array(1)); }),
      uuidReceiver: errorShape(function () { randomUUID.call({}); }),
    },
  };
}
