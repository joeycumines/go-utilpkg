function webPerformance() {
  const first = performance.now();
  const second = performance.now();
  function errorShape(callback) {
    try { callback(); return { ok: true }; }
    catch (error) { return { ok: false, name: error.name, code: error.code }; }
  }
  const now = performance.now;
  const toJSON = performance.toJSON;
  const timeOriginGetter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(performance), "timeOrigin").get;
  const json = performance.toJSON();
  return {
    nowFinite: Number.isFinite(first) && Number.isFinite(second),
    monotonic: second >= first,
    nonnegative: first >= 0,
    originFinite: Number.isFinite(performance.timeOrigin),
    identity: performance.timeOrigin + first <= Date.now() + 1000,
    receivers: [errorShape(function () { now.call({}); }), errorShape(function () { timeOriginGetter.call({}); }), errorShape(function () { toJSON.call({}); })],
    toJSON: [Object.keys(json), json.timeOrigin === performance.timeOrigin, json !== performance.toJSON()],
    removedTimeline: [typeof performance.mark, typeof performance.measure, typeof performance.getEntries, typeof performance.clearMarks],
  };
}
