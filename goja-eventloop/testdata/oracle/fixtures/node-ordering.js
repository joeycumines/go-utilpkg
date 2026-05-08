async function nodeOrdering() {
  const checkpoint = [];
  await new Promise(function (resolve) {
    setImmediate(function () {
      process.nextTick(function () { checkpoint.push("nextTick"); });
      queueMicrotask(function () { checkpoint.push("microtask"); });
      Promise.resolve().then(function () { checkpoint.push("promise"); });
      process.nextTick(function () {
        checkpoint.push("nextTick-2");
        queueMicrotask(function () { checkpoint.push("nested-microtask"); });
      });
      setImmediate(resolve);
    });
  });

  const phases = [];
  await new Promise(function (resolve) {
    setTimeout(function timerPhase() {
      setTimeout(function nestedTimer() {
        phases.push("timer");
        resolve();
      }, 0);
      setImmediate(function nestedImmediate() { phases.push("immediate"); });
    }, 0);
  });

  const recursive = [];
  await new Promise(function (resolve) {
    process.nextTick(function firstTick() {
      recursive.push("tick-1");
      process.nextTick(function secondTick() { recursive.push("tick-2"); });
      Promise.resolve().then(function promiseJob() { recursive.push("promise"); });
    });
    setImmediate(function finish() { resolve(); });
  });
  return { checkpoint: checkpoint, phases: phases, recursive: recursive };
}
