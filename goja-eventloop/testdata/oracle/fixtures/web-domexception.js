function webDOMException() {
  function descriptor(owner, name) {
    const value = Object.getOwnPropertyDescriptor(owner, name);
    return {
      configurable: value.configurable,
      enumerable: value.enumerable,
      writable: Object.prototype.hasOwnProperty.call(value, "writable") ? value.writable : undefined,
      getter: typeof value.get,
      setter: typeof value.set,
    };
  }
  function receiverError(name) {
    try {
      Object.getOwnPropertyDescriptor(DOMException.prototype, name).get.call({});
      return { ok: true };
    } catch (error) {
      return { ok: false, name: error.name, code: error.code };
    }
  }

  const error = new DOMException("message", "AbortError");
  const defaults = new DOMException();
  const constant = Object.getOwnPropertyDescriptor(DOMException, "ABORT_ERR");
  return {
    values: [error.name, error.message, error.code, String(error), defaults.name, defaults.message, defaults.code],
    brands: [error instanceof DOMException, error instanceof Error, Object.prototype.toString.call(error), Object.getPrototypeOf(DOMException.prototype) === Error.prototype],
    prototypeDescriptors: {
      name: descriptor(DOMException.prototype, "name"),
      message: descriptor(DOMException.prototype, "message"),
      code: descriptor(DOMException.prototype, "code"),
    },
    constantDescriptor: [constant.value, constant.writable, constant.enumerable, constant.configurable],
    prototypeConstant: DOMException.prototype.ABORT_ERR,
    receivers: [receiverError("name"), receiverError("message"), receiverError("code")],
    expandos: (function () { error.extra = 1; return [error.extra, Object.keys(error)]; })(),
  };
}
