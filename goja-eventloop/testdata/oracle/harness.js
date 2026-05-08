(function installOracleHarness(global) {
  "use strict";

  const ObjectCtor = Object;
  const ArrayCtor = Array;
  const PromiseCtor = Promise;
  const JSONCtor = JSON;
  const ReflectCtor = Reflect;
  const objectPrototype = ObjectCtor.prototype;
  const objectGetOwnPropertySymbols = ObjectCtor.getOwnPropertySymbols;
  const missingSymbolKeys = ObjectCtor.create(null);
  const hasOwn = Function.prototype.call.bind(objectPrototype.hasOwnProperty);
  const objectToString = Function.prototype.call.bind(objectPrototype.toString);
  let auditBaseline = [];
  let auditSpecs = [];
  let constructionAudits = [];
  let restoreActions = [];

  function functionShape(value, complete) {
    const result = { name: String(value.name), length: Number(value.length) };
    if (!complete) return result;
    let constructable = true;
    try { ReflectCtor.construct(ObjectCtor, [], value); }
    catch (_) { constructable = false; }
    const prototype = ObjectCtor.getOwnPropertyDescriptor(value, "prototype");
    result.constructable = constructable;
	result.ownPropertyNames = ObjectCtor.getOwnPropertyNames(value).sort();
    result.ownPrototype = prototype ? {
      exists: true,
      configurable: Boolean(prototype.configurable),
      enumerable: Boolean(prototype.enumerable),
      writable: Boolean(prototype.writable),
    } : { exists: false };
    return result;
  }

  function errorShape(error) {
    const value = {
      name: error && typeof error.name === "string" ? error.name : "Error",
      message: error && typeof error.message === "string" ? error.message : String(error),
    };
    if (error && (typeof error.code === "string" || typeof error.code === "number")) {
      value.code = error.code;
    }
    return value;
  }

  function normalize(value, seen) {
    if (value === undefined) return { $type: "undefined" };
    if (value === null || typeof value === "string" || typeof value === "boolean") return value;
    if (typeof value === "number") {
      if (Number.isNaN(value)) return { $type: "number", value: "NaN" };
      if (value === Infinity) return { $type: "number", value: "Infinity" };
      if (value === -Infinity) return { $type: "number", value: "-Infinity" };
      if (ObjectCtor.is(value, -0)) return { $type: "number", value: "-0" };
      return value;
    }
    if (typeof value === "bigint") return { $type: "bigint", value: String(value) };
    if (typeof value === "symbol") return { $type: "symbol", value: String(value) };
    if (typeof value === "function") return { $type: "function", shape: functionShape(value) };
    if (value instanceof Error) return { $type: "error", value: errorShape(value) };

    const stack = seen || [];
    if (stack.indexOf(value) !== -1) return { $type: "circular" };
    stack.push(value);
    let result;
    if (ArrayCtor.isArray(value)) {
      result = value.map(function normalizeArrayEntry(entry) { return normalize(entry, stack); });
    } else {
      result = {};
      ObjectCtor.keys(value).sort().forEach(function normalizeObjectEntry(key) {
        result[key] = normalize(value[key], stack);
      });
    }
    stack.pop();
    return result;
  }

  function setup(spec, input) {
    const value = spec || {};
    restoreActions = [];
    const globals = (value.globals || []).map(function captureGlobal(name) {
      return { name: name, prior: ObjectCtor.getOwnPropertyDescriptor(global, name) };
    });
    globals.forEach(function installGlobal(entry) {
      const name = entry.name;
      const prior = entry.prior;
      restoreActions.push(function restoreGlobal() {
        if (prior) ObjectCtor.defineProperty(global, name, prior);
        else delete global[name];
      });
      ObjectCtor.defineProperty(global, name, {
        configurable: true,
        enumerable: true,
        writable: true,
        value: { __oracleSentinel: name },
      });
    });
    (value.members || []).forEach(function installMember(member) {
      let owner = global[member.object];
      if ((typeof owner !== "object" && typeof owner !== "function") || owner === null) {
        owner = {};
        global[member.object] = owner;
      }
      const prior = ObjectCtor.getOwnPropertyDescriptor(owner, member.property);
      restoreActions.push(function restoreMember() {
        if (prior) ObjectCtor.defineProperty(owner, member.property, prior);
        else delete owner[member.property];
      });
      ObjectCtor.defineProperty(owner, member.property, {
        configurable: true,
        enumerable: true,
        writable: true,
        value: member.value,
      });
    });

    (value.brandPairs || []).forEach(function installBrandPair(pair) {
      const constructorPrior = ObjectCtor.getOwnPropertyDescriptor(global, pair.constructor);
      const singletonPrior = ObjectCtor.getOwnPropertyDescriptor(global, pair.singleton);
      restoreActions.push(function restoreBrandPair() {
        if (singletonPrior) ObjectCtor.defineProperty(global, pair.singleton, singletonPrior);
        else delete global[pair.singleton];
        if (constructorPrior) ObjectCtor.defineProperty(global, pair.constructor, constructorPrior);
        else delete global[pair.constructor];
      });

      const ForeignBrand = function ForeignBrand() {};
      ObjectCtor.defineProperty(ForeignBrand, "name", { configurable: true, value: pair.constructor });
      ObjectCtor.defineProperty(ForeignBrand, "__oracleSentinel", {
        configurable: true,
        enumerable: true,
        writable: true,
        value: pair.constructor,
      });
      ObjectCtor.defineProperty(ForeignBrand.prototype, Symbol.toStringTag, {
        configurable: true,
        enumerable: false,
        writable: false,
        value: pair.constructor,
      });
      (pair.methods || []).forEach(function installBrandMethod(name) {
        ObjectCtor.defineProperty(ForeignBrand.prototype, name, {
          configurable: true,
          enumerable: true,
          writable: true,
          value: function preservedHostMethod() {},
        });
      });
      (pair.accessors || []).forEach(function installBrandAccessor(name) {
        ObjectCtor.defineProperty(ForeignBrand.prototype, name, {
          configurable: true,
          enumerable: true,
          get: function preservedHostGetter() {},
        });
      });
      const singleton = ObjectCtor.create(ForeignBrand.prototype);
      ObjectCtor.defineProperty(singleton, "__oracleSentinel", {
        configurable: true,
        enumerable: true,
        writable: true,
        value: pair.sentinel,
      });
      ObjectCtor.defineProperty(global, pair.constructor, {
        configurable: true,
        enumerable: true,
        writable: true,
        value: ForeignBrand,
      });
      ObjectCtor.defineProperty(global, pair.singleton, {
        configurable: true,
        enumerable: true,
        writable: true,
        value: singleton,
      });
    });
    auditSpecs = (input && input.audits) || [];
    constructionAudits = [];
    const cleanup = [];
    try {
      auditBaseline = auditSpecs.map(function captureBaseline(audit) {
        return snapshotAudit(audit, cleanup);
      });
    } finally {
      for (let index = cleanup.length - 1; index >= 0; index -= 1) cleanup[index]();
    }
  }

  function restore() {
    for (let index = restoreActions.length - 1; index >= 0; index -= 1) restoreActions[index]();
    restoreActions = [];
  }

  function propertyKey(segment, owner) {
    if (segment === "@@toPrimitive") return Symbol.toPrimitive;
    if (segment === "@@dispose") return Symbol.dispose;
    if (segment === "@@toStringTag") return Symbol.toStringTag;
    if (segment === "@@nodejs.util.inspect.custom") return Symbol.for("nodejs.util.inspect.custom");
    if (segment.indexOf("@@") === 0) {
      const description = segment.slice(2);
      const symbols = ReflectCtor.apply(objectGetOwnPropertySymbols, ObjectCtor, [owner]);
      for (let index = 0; index < symbols.length; index += 1) {
        if (symbols[index].description === description) return symbols[index];
      }
      if (!hasOwn(missingSymbolKeys, description)) {
        missingSymbolKeys[description] = Symbol("oracle.missing." + description);
      }
      return missingSymbolKeys[description];
    }
    return segment;
  }

  function prototypeName(value) {
    const prototype = ObjectCtor.getPrototypeOf(value);
    if (prototype === null) return "null";
    const constructor = prototype.constructor;
    if (constructor && typeof constructor.name === "string" && constructor.name !== "") {
      return constructor.name;
    }
    return objectToString(prototype).slice(8, -1);
  }

  function descriptor(owner, key) {
    let current = owner;
    let depth = 0;
    while (current !== null) {
      const value = ObjectCtor.getOwnPropertyDescriptor(current, key);
      if (value) return { value: value, depth: depth };
      current = ObjectCtor.getPrototypeOf(current);
      depth += 1;
    }
    return null;
  }

  function resolveRoot(name, cleanup) {
    if (name === "global") return global;
    if (name === "processPrototype") {
      if ((typeof global.process !== "object" && typeof global.process !== "function") || global.process === null) return undefined;
      const processPrototype = ObjectCtor.getPrototypeOf(global.process);
      const constructor = processPrototype && ObjectCtor.getOwnPropertyDescriptor(processPrototype, "constructor");
      if (!constructor || typeof constructor.value !== "function" || constructor.value.name !== "process") return undefined;
      return processPrototype;
    }
    if (name === "processEmitterPrototype") {
      if ((typeof global.process !== "object" && typeof global.process !== "function") || global.process === null) return undefined;
      const processPrototype = ObjectCtor.getPrototypeOf(global.process);
      const constructor = processPrototype && ObjectCtor.getOwnPropertyDescriptor(processPrototype, "constructor");
      if (!constructor || typeof constructor.value !== "function" || constructor.value.name !== "process") return undefined;
      return processPrototype === null ? undefined : ObjectCtor.getPrototypeOf(processPrototype);
    }
    if (name === "timeoutPrototype") {
      if (typeof global.setTimeout !== "function") return undefined;
      const handle = global.setTimeout(function oracleTimeoutGuard() {}, 60000);
      cleanup.push(function clearOracleTimeout() { global.clearTimeout(handle); });
      return ObjectCtor.getPrototypeOf(handle);
    }
    if (name === "timeoutInstance") {
      if (typeof global.setTimeout !== "function") return undefined;
      const handle = global.setTimeout(function oracleTimeoutInstanceGuard() {}, 60000);
      cleanup.push(function clearOracleTimeoutInstance() { global.clearTimeout(handle); });
      return handle;
    }
    if (name === "immediatePrototype") {
      if (typeof global.setImmediate !== "function") return undefined;
      const handle = global.setImmediate(function oracleImmediateGuard() {});
      cleanup.push(function clearOracleImmediate() { global.clearImmediate(handle); });
      return ObjectCtor.getPrototypeOf(handle);
    }
    if (name === "immediateInstance") {
      if (typeof global.setImmediate !== "function") return undefined;
      const handle = global.setImmediate(function oracleImmediateInstanceGuard() {});
      cleanup.push(function clearOracleImmediateInstance() { global.clearImmediate(handle); });
      return handle;
    }
    if (name === "eventInstance") {
      if (typeof global.Event !== "function") return undefined;
      return new global.Event("oracle");
    }
    throw new Error("unknown surface root: " + name);
  }

  function auditKey(key) {
    if (key === Symbol.toPrimitive) return "@@toPrimitive";
    if (key === Symbol.dispose) return "@@dispose";
    if (key === Symbol.toStringTag) return "@@toStringTag";
    if (key === Symbol.for("nodejs.util.inspect.custom")) return "@@nodejs.util.inspect.custom";
    return typeof key === "symbol" ? String(key) : String(key);
  }

  function auditOwner(spec, cleanup) {
    let owner = resolveRoot(spec.root, cleanup);
    for (let index = 0; index < spec.segments.length; index += 1) {
      if (owner === null || owner === undefined) return undefined;
      owner = owner[propertyKey(spec.segments[index], owner)];
    }
    return owner;
  }

  function ignoredAuditKey(owner, key) {
    if (typeof key !== "string") return false;
    if (key === "__gojaEventloopOracle" || key.indexOf("__oracle") === 0) return true;
    if (key === "constructor") return true;
    return typeof owner === "function" && (key === "length" || key === "name" || key === "prototype" || key === "arguments" || key === "caller");
  }

  function snapshotAudit(spec, cleanup) {
    const owner = auditOwner(spec, cleanup);
    const descriptors = [];
    if ((typeof owner === "object" && owner !== null) || typeof owner === "function") {
      ReflectCtor.ownKeys(owner).forEach(function captureDescriptor(key) {
        if (!ignoredAuditKey(owner, key)) descriptors.push({ key: key, token: auditKey(key), descriptor: ObjectCtor.getOwnPropertyDescriptor(owner, key) });
      });
    }
    return { id: spec.id, path: spec.path, exists: owner !== undefined && owner !== null, descriptors: descriptors };
  }

  function descriptorEqual(left, right) {
    if (!left || !right) return left === right;
    return left.configurable === right.configurable &&
      left.enumerable === right.enumerable &&
      left.writable === right.writable &&
      ObjectCtor.is(left.value, right.value) &&
      ObjectCtor.is(left.get, right.get) &&
      ObjectCtor.is(left.set, right.set);
  }

  function findAuditDescriptor(snapshot, token) {
    for (let index = 0; index < snapshot.descriptors.length; index += 1) {
      if (snapshot.descriptors[index].token === token) return snapshot.descriptors[index].descriptor;
    }
    return undefined;
  }

  function observeAudits(specs, cleanup) {
    return (specs || []).map(function observeAudit(spec, index) {
      const before = auditBaseline[index] || { descriptors: [] };
      const after = snapshotAudit(spec, cleanup);
      const tokens = {};
      before.descriptors.forEach(function addBefore(entry) { tokens[entry.token] = true; });
      after.descriptors.forEach(function addAfter(entry) { tokens[entry.token] = true; });
      const changes = ObjectCtor.keys(tokens).filter(function changed(token) {
        return !descriptorEqual(findAuditDescriptor(before, token), findAuditDescriptor(after, token));
      }).sort();
      return { id: spec.id, path: spec.path, changes: changes };
    });
  }

  function checkpoint() {
    const cleanup = [];
    try {
      constructionAudits = observeAudits(auditSpecs, cleanup);
      auditBaseline = auditSpecs.map(function captureBindBaseline(audit) { return snapshotAudit(audit, cleanup); });
      return constructionAudits;
    } finally {
      for (let index = cleanup.length - 1; index >= 0; index -= 1) cleanup[index]();
    }
  }

  function observeSurface(spec, root) {
    let owner = root;
    for (let index = 0; index + 1 < spec.segments.length; index += 1) {
      if (owner === null || owner === undefined) return { id: spec.id, path: spec.path, exists: false };
      owner = owner[propertyKey(spec.segments[index], owner)];
    }
    if (owner === null || owner === undefined) return { id: spec.id, path: spec.path, exists: false };
    const key = propertyKey(spec.segments[spec.segments.length - 1], owner);
    const found = descriptor(owner, key);
    if (!found) return { id: spec.id, path: spec.path, exists: false };

    const raw = found.value;
    const observation = {
      id: spec.id,
      path: spec.path,
      exists: true,
      kind: hasOwn(raw, "value") ? (raw.value === null ? "null" : typeof raw.value) : "accessor",
      descriptor: {
        depth: found.depth,
        configurable: Boolean(raw.configurable),
        enumerable: Boolean(raw.enumerable),
      },
    };
    if (hasOwn(raw, "value")) {
      observation.descriptor.writable = Boolean(raw.writable);
      if (typeof raw.value === "function") observation.function = functionShape(raw.value, spec.completeFunctionShape);
      if ((typeof raw.value === "object" && raw.value !== null) || typeof raw.value === "function") {
        observation.prototype = prototypeName(raw.value);
      }
      if (raw.value && typeof raw.value.__oracleSentinel === "string") {
        observation.sentinel = raw.value.__oracleSentinel;
      }
      if (spec.valueMode !== "type" &&
          (typeof raw.value === "number" || typeof raw.value === "string" || typeof raw.value === "boolean")) {
        observation.value = raw.value;
      }
    } else {
      if (typeof raw.get === "function") observation.descriptor.getter = functionShape(raw.get, spec.completeFunctionShape);
      if (typeof raw.set === "function") observation.descriptor.setter = functionShape(raw.set, spec.completeFunctionShape);
    }
    return observation;
  }

  function surfaces(specs) {
    const cleanup = [];
    const roots = {};
    try {
      return { surfaces: (specs || []).map(function inspect(spec) {
        if (!hasOwn(roots, spec.root)) roots[spec.root] = resolveRoot(spec.root, cleanup);
        return observeSurface(spec, roots[spec.root]);
      }) };
    } finally {
      for (let index = cleanup.length - 1; index >= 0; index -= 1) cleanup[index]();
    }
  }

  function surfaceFixture(input) {
    const value = input || {};
    const cleanup = [];
    try {
      const roots = {};
      const observed = (value.surfaces || []).map(function inspect(spec) {
        if (!hasOwn(roots, spec.root)) roots[spec.root] = resolveRoot(spec.root, cleanup);
        return observeSurface(spec, roots[spec.root]);
      });
      return { surfaces: observed, constructionAudits: constructionAudits, audits: observeAudits(value.audits, cleanup) };
    } finally {
      for (let index = cleanup.length - 1; index >= 0; index -= 1) cleanup[index]();
    }
  }

  function run(fixture, input) {
    if (typeof fixture !== "function") return PromiseCtor.reject(new TypeError("fixture must evaluate to a function"));
    return PromiseCtor.resolve().then(function callFixture() {
      return fixture(api, input || {});
    }).then(
      function fixtureFulfilled(value) { return { ok: true, value: value }; },
      function fixtureRejected(error) { return { ok: false, error: errorShape(error) }; }
    );
  }

  function encode(value) {
    return JSONCtor.stringify(normalize(value));
  }

  const api = ObjectCtor.freeze({ checkpoint: checkpoint, encode: encode, normalize: normalize, restore: restore, run: run, setup: setup, surfaces: surfaces, surfaceFixture: surfaceFixture });
  ObjectCtor.defineProperty(global, "__gojaEventloopOracle", {
    configurable: false,
    enumerable: false,
    writable: false,
    value: api,
  });
})(globalThis);
