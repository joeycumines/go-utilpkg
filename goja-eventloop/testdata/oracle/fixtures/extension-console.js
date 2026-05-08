function extensionConsole() {
  function capture(callback) {
    return globalThis.__oracleCaptureConsole(callback);
  }

  function normalizeElapsed(output) {
    return output.replace(/(^|\n)(clock: )\d+(?:\.\d+)?ms/g, "$1$2<elapsed>ms");
  }

  const timer = normalizeElapsed(capture(function exerciseTimers() {
    console.time("clock");
    console.time("clock");
    console.timeLog("clock", "payload", 7);
    console.timeLog("missing-log");
    console.timeEnd("clock");
    console.timeEnd("missing-end");
  }));

  const count = capture(function exerciseCounters() {
    console.count("jobs");
    console.count("jobs");
    console.countReset("jobs");
    console.count("jobs");
    console.countReset("missing");
  });

  const assertTruthy = capture(function exerciseTruthyAssertion() {
    console.assert(true, "must remain silent");
  });
  const assertFalsy = capture(function exerciseFalsyAssertion() {
    console.assert(false, "payload", 7);
  });

  const tableOutput = capture(function exerciseTable() {
    console.table([
      { name: "alpha", score: 1 },
      { name: "beta", score: 2 },
    ], ["name"]);
  });

  const groupOutput = capture(function exerciseGroups() {
    console.group("outer");
    console.groupCollapsed("inner");
    console.groupEnd();
    console.dir("nested");
    console.groupEnd();
    console.dir("root");
  });
  const groupLines = groupOutput.trimEnd().split("\n");

  const traceOutput = capture(function exerciseTrace() {
    console.trace("trace-marker");
  });
  const traceLines = traceOutput.trimEnd().split("\n");

  const clear = capture(function exerciseClear() {
    console.clear();
  });
  const dir = capture(function exerciseDir() {
    console.dir(null);
    console.dir(undefined);
  });

  const originalConsole = globalThis.console;
  const thrown = {};
  let captureExceptionIdentity = false;
  try {
    capture(function throwCapturedIdentity() {
      throw thrown;
    });
  } catch (error) {
    captureExceptionIdentity = error === thrown;
  }

  const sequentialFirst = capture(function firstSequentialCapture() {
    console.count("oracle-sequential");
  });
  const sequentialSecond = capture(function secondSequentialCapture() {
    console.count("oracle-sequential");
  });
  let nestedInner;
  const nestedOuter = capture(function outerCapture() {
    console.count("oracle-nested");
    nestedInner = capture(function innerCapture() {
      console.count("oracle-nested");
    });
    console.count("oracle-nested");
  });

  return {
    time: typeof console.time,
    timeEnd: typeof console.timeEnd,
    timeLog: typeof console.timeLog,
    count: typeof console.count,
    countReset: typeof console.countReset,
    assert: typeof console.assert,
    table: typeof console.table,
    group: typeof console.group,
    groupCollapsed: typeof console.groupCollapsed,
    groupEnd: typeof console.groupEnd,
    trace: typeof console.trace,
    clear: typeof console.clear,
    dir: typeof console.dir,
    timer: timer,
    countOutput: count,
    assertOutput: {
      truthyOutput: assertTruthy,
      falsyOutput: assertFalsy,
    },
    tableOutput: tableOutput,
    groupOutput: {
      lines: groupLines.map(function trimGroupLine(line) { return line.trimStart(); }),
      leadingSpaces: groupLines.map(function countLeadingSpaces(line) {
        return line.length - line.trimStart().length;
      }),
    },
    traceOutput: {
      firstLine: traceLines[0],
      hasFrame: traceLines.slice(1).some(function isFrame(line) {
        return /^\s+at /.test(line);
      }),
    },
    clearOutput: clear,
    dirOutput: dir,
    captureExceptionIdentity: captureExceptionIdentity,
    captureRestoredConsole: globalThis.console === originalConsole,
    captureRouting: {
      nestedInner: nestedInner,
      nestedOuter: nestedOuter,
      sequentialFirst: sequentialFirst,
      sequentialSecond: sequentialSecond,
    },
  };
}
