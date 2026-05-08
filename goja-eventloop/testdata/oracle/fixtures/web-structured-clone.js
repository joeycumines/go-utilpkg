function webStructuredClone() {
  function errorShape(callback) {
    try { callback(); return { ok: true }; }
    catch (error) { return { ok: false, name: error.name, code: error.code, constructor: error.constructor.name }; }
  }

  const date = new Date(1234);
  date.expando = "excluded";
  const regexp = new RegExp("a+", "gimsuy");
  regexp.lastIndex = 2;
  regexp.expando = "excluded";
  const error = new TypeError("message", { cause: 7 });
  error.expando = "excluded";
  const source = {
    array: [1, { value: "x" }],
    date: date,
    regexp: regexp,
    error: error,
    map: new Map([["a", { value: 1 }]]),
    set: new Set([2]),
  };
  source.self = source;
  const clone = structuredClone(source);

  const transferSource = new ArrayBuffer(4);
  new Uint8Array(transferSource).set([1, 2, 3, 4]);
  const transferred = structuredClone({ buffer: transferSource }, { transfer: [transferSource] });
  const duplicate = new ArrayBuffer(1);
  const unreachable = new ArrayBuffer(2);
  const unreachableResult = structuredClone({ value: 1 }, { transfer: [unreachable] });

  const controller = new AbortController();
  const timeout = setTimeout(function () {}, 1000);
  const immediate = setImmediate(function () {});
  const platformErrors = [
    new Event("sample"),
    new EventTarget(),
    controller,
    controller.signal,
    performance,
    crypto,
  ].map(function (value) {
    return errorShape(function () { structuredClone({ value: value }); });
  });
  const activeHandleErrors = [
    errorShape(function () { structuredClone(timeout); }),
    errorShape(function () { structuredClone(immediate); }),
  ];
  clearTimeout(timeout);
  clearImmediate(immediate);
  const timeoutClone = structuredClone(timeout);
  const immediateClone = structuredClone(immediate);

  const bigint = Object(7n);
  bigint.extra = { marker: 1 };
  const bigintClone = structuredClone(bigint);
  const boxedSymbol = errorShape(function () { structuredClone(Object(Symbol("sample"))); });

  const sparse = [];
  sparse.length = 4294967295;
  sparse[4294967294] = "tail";
  const sparseClone = structuredClone(sparse);

  let messageGets = 0;
  const undefinedMessage = new Error("source");
  Object.defineProperty(undefinedMessage, "message", { value: undefined, configurable: true });
  const undefinedMessageClone = structuredClone(undefinedMessage);
  const accessorMessage = new Error();
  Object.defineProperty(accessorMessage, "message", {
    configurable: true,
    get: function () { messageGets += 1; return "wrong"; },
  });
  const accessorMessageClone = structuredClone(accessorMessage);
  const objectStack = new Error("stack");
  Object.defineProperty(objectStack, "stack", { value: { wrong: true }, configurable: true });
  const objectStackClone = structuredClone(objectStack);

  const detached = transferSource;
  return {
    missing: errorShape(function () { structuredClone(); }),
    uncloneable: [
      errorShape(function () { structuredClone(function () {}); }),
      errorShape(function () { structuredClone(Symbol("x")); }),
    ],
    transferErrors: [
      errorShape(function () { structuredClone({}, { transfer: {} }); }),
      errorShape(function () { structuredClone({}, { transfer: [{}] }); }),
      errorShape(function () { structuredClone({}, { transfer: [duplicate, duplicate] }); }),
      errorShape(function () { structuredClone(detached); }),
    ],
    distinct: clone !== source && clone.array !== source.array && clone.map.get("a") !== source.map.get("a"),
    circular: clone.self === clone,
    platformErrors: platformErrors,
    timerHandles: [
      activeHandleErrors,
      timeoutClone !== timeout,
      timeoutClone._onTimeout === null,
      immediateClone !== immediate,
      immediateClone._onImmediate === null,
    ],
    wrappers: [
      bigintClone !== bigint,
      bigintClone.valueOf() === 7n,
      bigintClone.extra === undefined,
      boxedSymbol,
    ],
    sparse: [sparseClone.length, sparseClone[4294967294]],
    errorData: [
      Object.hasOwn(undefinedMessageClone, "message"),
      undefinedMessageClone.message,
      Object.hasOwn(accessorMessageClone, "message"),
      messageGets,
      Object.hasOwn(objectStackClone, "stack"),
      objectStackClone.stack,
    ],
    slots: {
      date: [clone.date.getTime(), clone.date.expando],
      regexp: [clone.regexp.source, clone.regexp.flags, clone.regexp.lastIndex, clone.regexp.expando],
      error: [clone.error.constructor.name, clone.error.name, clone.error.message, clone.error.cause, clone.error.expando],
      map: clone.map.get("a").value,
      set: clone.set.has(2),
    },
    transferred: [transferSource.byteLength, transferred.buffer.byteLength, Array.from(new Uint8Array(transferred.buffer))],
    unreachable: [unreachable.byteLength, unreachableResult.value],
  };
}
