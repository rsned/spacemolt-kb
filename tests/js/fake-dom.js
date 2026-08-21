'use strict';
// fake-dom.js — a minimal fake `doc`/`win`/`els` for driving createPlayer
// (holotable-player.js) through its dependency-injection seam in Node.
//
// This is not a DOM emulator. It implements exactly what createPlayer,
// railBlock, and holotable.js's draw functions touch: element creation,
// appendChild/removeChild/textContent (enough for the rail's list-of-<li>
// building and its rolling-window trim), addEventListener/dispatch (enough
// to simulate a user click or slider drag), and a canvas 2d context that
// no-ops every drawing call so drawStatic/drawShips can run for real without
// a browser.

// fakeCtx is a Proxy over a plain object: any property read that isn't
// already a stored value returns a no-op function, and any assignment (e.g.
// ctx.fillStyle = '#fff') is stored and forgotten. holotable.js's draw
// functions call dozens of Canvas2D methods and set dozens of style
// properties; a Proxy means this file doesn't have to enumerate them, and
// naming them all here as fields on a canvas element you did not build.
function fakeCtx() {
  const store = {};
  return new Proxy(store, {
    get(target, prop) {
      if (prop in target) return target[prop];
      return function () { /* no-op draw call */ };
    },
    set(target, prop, value) {
      target[prop] = value;
      return true;
    },
  });
}

// fakeElement covers what createPlayer needs from any DOM node it touches:
// canvases (getContext, width/height, clientWidth/clientHeight), the rail
// list (appendChild/removeChild/childElementCount/firstChild), form
// controls (value, addEventListener), and text nodes (textContent, which —
// like the real DOM — clears any children when set).
function fakeElement(tag) {
  let text = '';
  const el = {
    tagName: String(tag || '').toUpperCase(),
    type: '',
    children: [],
    listeners: {},
    value: '',
    scrollTop: 0,
    scrollHeight: 0,
    clientHeight: 0,
    clientWidth: 0,
    width: 0,
    height: 0,
    max: '',

    appendChild(child) {
      this.children.push(child);
      return child;
    },
    removeChild(child) {
      const i = this.children.indexOf(child);
      if (i >= 0) this.children.splice(i, 1);
      return child;
    },
    get firstChild() {
      return this.children[0] || null;
    },
    get childElementCount() {
      return this.children.length;
    },
    addEventListener(type, fn) {
      (this.listeners[type] = this.listeners[type] || []).push(fn);
    },
    // Not a real DOM dispatch — just calls every listener registered for
    // `type`, which is all createPlayer's own click/input/keydown handlers
    // need to run.
    dispatch(type, evt) {
      for (const fn of this.listeners[type] || []) fn(evt || {target: this});
    },
    getContext() {
      return fakeCtx();
    },
  };
  Object.defineProperty(el, 'textContent', {
    get() { return text; },
    set(v) { text = v; el.children.length = 0; },
  });
  return el;
}

function fakeDoc() {
  return {createElement: (tag) => fakeElement(tag)};
}

// fakeWin exposes the one requestAnimationFrame callback createPlayer has
// scheduled at any time as `win.rafCb`, so a test can step the playback
// loop by hand — `win.rafCb(now)` — instead of needing a real animation
// frame clock.
function fakeWin() {
  const listeners = {};
  let rafId = 0;
  return {
    devicePixelRatio: 1,
    rafCb: null,
    requestAnimationFrame(cb) {
      this.rafCb = cb;
      return ++rafId;
    },
    cancelAnimationFrame() {
      this.rafCb = null;
    },
    addEventListener(type, fn) {
      (listeners[type] = listeners[type] || []).push(fn);
    },
    performance: {now: () => 0},
  };
}

// A minimal but real replay: enough zones, bounds, one participant and a run
// of ticks for holotable.js's actual layoutTable/drawStatic/drawShips to run
// unmocked (only ctx is faked). `frameCount` controls how many ticks are
// available to advance through.
function fakeReplay(frameCount) {
  const frames = [];
  for (let i = 0; i < frameCount; i++) {
    frames.push({
      tick: 1000 + i,
      ships: [{player_id: 'a', x: i * 0.01, y: 0, zone: 'outer'}],
      chatter: [], moves: [], kills: [],
    });
  }
  return {
    zones: ['outer', 'mid', 'inner', 'engaged'],
    centre: {x: 0, y: 0},
    bounds: {x_min: -1, x_max: 1, y_min: -1, y_max: 1},
    sides: [],
    participants: [{player_id: 'a', username: 'Test Hull', side_id: 1, ship_class: 'shard'}],
    frames,
  };
}

// A harness bundling everything createPlayer's cfg needs, wired for Node.
function makeHarness(frameCount) {
  const doc = fakeDoc();
  const win = fakeWin();
  const canvas = fakeElement('canvas');
  canvas.clientWidth = 800;
  canvas.clientHeight = 600;
  const els = {
    canvas,
    rail: fakeElement('aside'),
    chatter: fakeElement('ol'),
    playPause: fakeElement('button'),
    stepBack: fakeElement('button'),
    stepFwd: fakeElement('button'),
    scrub: fakeElement('input'),
    speed: fakeElement('select'),
    readout: fakeElement('span'),
    tick: fakeElement('span'),
  };
  return {doc, win, els, replay: fakeReplay(frameCount), hulls: {}};
}

module.exports = {fakeCtx, fakeElement, fakeDoc, fakeWin, fakeReplay, makeHarness};
