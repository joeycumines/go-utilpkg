function webBase64() {
  function errorShape(callback) {
    try { callback(); return { ok: true }; }
    catch (error) { return { ok: false, name: error.name, code: error.code, constructor: error.constructor.name }; }
  }
  const encoded = btoa("hello\u0000world");
  return {
    encoded: encoded,
    decoded: atob(encoded),
    forgiving: [atob(" Y W J j \n"), atob("YWI"), atob("YQ==")],
    empty: [atob(""), btoa("")],
    errors: [
      errorShape(function () { btoa("\u20ac"); }),
      errorShape(function () { atob("A"); }),
      errorShape(function () { atob("A==="); }),
      errorShape(function () { atob("***"); }),
    ],
  };
}
