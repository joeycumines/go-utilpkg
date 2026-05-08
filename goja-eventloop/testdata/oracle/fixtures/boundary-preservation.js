function boundaryPreservation() {
  const names = ["URL", "URLSearchParams", "TextEncoder", "TextDecoder", "Blob", "Headers", "FormData", "localStorage", "sessionStorage", "fetch", "require", "Crypto", "crypto", "Performance", "performance"];
  const result = {};
  names.forEach(function (name) { result[name] = globalThis[name] && globalThis[name].__oracleSentinel; });
  result.cryptoBrand = Object.getPrototypeOf(globalThis.crypto) === globalThis.Crypto.prototype;
  result.performanceBrand = Object.getPrototypeOf(globalThis.performance) === globalThis.Performance.prototype;
  result.consoleMember = globalThis.console && globalThis.console.__oracleMember;
  result.processMember = globalThis.process && globalThis.process.__oracleMember;
  return result;
}
