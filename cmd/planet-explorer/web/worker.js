// Dedicated Worker that hosts the planet-explorer wasm module so heavy
// computes never block the page's main thread. Protocol:
//   main -> worker: {id, name, args}   (name = a wasm-exported global)
//   worker -> main: {type:'ready'}                       once, after boot
//                   {type:'progress', id, stage, i, n}   during a call
//                   {type:'result', id, result}          per call (Uint8Array
//                                                        buffers transferred)
//                   {type:'result', id, error}           on thrown exception
// Calls run synchronously on this worker thread — messages arriving
// mid-compute queue in the worker's event loop, which serializes RPCs
// in send order for free.
importScripts('wasm_exec.js');

let currentId = 0;

// Defined BEFORE the wasm boots so main.go's registerProgressHooks
// finds it. Forwards Go pipeline progress tagged with the in-flight id.
self.__pxProgress = (stage, i, n) => {
  self.postMessage({ type: 'progress', id: currentId, stage, i, n });
};

const ready = (async () => {
  const go = new Go();
  const result = await WebAssembly.instantiateStreaming(fetch('wasm'), go.importObject);
  // go.run resolves only when the Go program exits (it never does —
  // main blocks on a channel). Exports are installed synchronously
  // before that first block, so we deliberately do not await it.
  go.run(result.instance);
  self.postMessage({ type: 'ready' });
})();
// A rejected boot must be loud: unhandled rejections in a Worker fire no
// event on the main thread's Worker object, so without this the page
// would spin forever with zero console output.
ready.catch((err) => {
  self.postMessage({ type: 'boot_error', error: String(err && err.stack || err) });
});

self.onmessage = async (e) => {
  const { id, name, args } = e.data;
  try {
    await ready;
  } catch (err) {
    self.postMessage({ type: 'result', id, error: 'wasm boot failed: ' + String(err) });
    return;
  }
  currentId = id;
  let result;
  try {
    result = self[name](...args);
  } catch (err) {
    self.postMessage({ type: 'result', id, error: String(err && err.stack || err) });
    return;
  }
  if (result instanceof Uint8Array) {
    self.postMessage({ type: 'result', id, result }, [result.buffer]);
  } else {
    self.postMessage({ type: 'result', id, result });
  }
};
