function boundaryAbsence() {
  const names = ["URL", "URLSearchParams", "TextEncoder", "TextDecoder", "Blob", "Headers", "FormData", "localStorage", "sessionStorage", "consumeIterable", "fetch"];
  const result = {};
  names.forEach(function (name) { result[name] = typeof globalThis[name]; });
  const timeout = setTimeout(function oracleBoundaryTimeoutGuard() {}, 60000);
  result.timeoutInspect = typeof timeout[Symbol.for("nodejs.util.inspect.custom")];
  clearTimeout(timeout);
  return result;
}
