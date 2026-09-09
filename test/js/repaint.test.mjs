// #804. Two Settings controls change what the board should show and did not
// repaint it, and returning to the Sessions tab did not either — so the board
// contradicted the setting just changed until the 5-second tick landed.
//
// The correct pattern was already in the file, one line below: the column
// controls go through a helper that repaints both surfaces.

import assert from "node:assert/strict";
import { test } from "node:test";

const APP = new URL("../../internal/web/static/app.js", import.meta.url);

// Reuse of live.test.mjs's approach: fake only the I/O boundary, and record
// which element each render writes into. What is asserted is "did the board get
// redrawn", which is the defect itself, not a claim about the HTML.
let instance = 0;
const flush = async (n = 8) => { for (let i = 0; i < n; i++) await new Promise((r) => setImmediate(r)); };

function harness(sessions) {
  const h = { writes: [], listeners: new Map(), intervals: [], now: Date.parse('2026-09-09T10:00:00Z') };
  // querySelectorAll hands back one stand-in per selector, so a listener attached
  // to a collection — the tab buttons, the Settings controls — is reachable. The
  // Settings handlers only exist once that panel has been rendered, which is the
  // whole reason this harness has to open it first.
  const element = (id) => {
    // `dataset` is a real object: app.js reads `b.dataset.tab` to decide which tab
    // a click means, so a stand-in that answers every property with itself makes
    // every tab the same tab.
    const ds = {};
    const node = new Proxy(function () {}, {
      get: (_t, k) =>
        k === "addEventListener" ? (ev, fn) => h.listeners.set(`${id}:${ev}`, fn)
          : k === "querySelectorAll" ? (sel) => [byId(`${id}${sel}`)]
            : k === "dataset" ? ds
              : k === "classList" || k === "style" ? node
                : k === "hidden" || k === "value" || k === "innerHTML" || k === "textContent" ? ""
                  : k === "children" || k === "options" ? []
                    : k === Symbol.toPrimitive || k === "toString" ? () => ""
                      : node,
      set: (_t, k, v) => { if (k === "innerHTML") h.writes.push(id); return true; },
      apply: () => node,
      has: () => true,
    });
    return node;
  };
  const els = new Map();
  const byId = (id) => { if (!els.has(id)) els.set(id, element(id)); return els.get(id); };
  globalThis.document = {
    getElementById: byId, querySelectorAll: () => [], createElement: () => byId("<new>"),
    addEventListener: (ev, fn) => h.listeners.set(`document:${ev}`, fn),
    documentElement: byId("<root>"), body: byId("<body>"),
  };
  globalThis.window = globalThis;
  globalThis.localStorage = { getItem: (k) => (k === "vigie_token" ? "t0k3n" : null), setItem() {}, removeItem() {} };
  globalThis.matchMedia = () => ({ matches: false, addEventListener() {} });
  globalThis.scrollTo = () => {};
  globalThis.location = { origin: "http://localhost:8080", protocol: "http:", hostname: "localhost" };
  globalThis.Notification = undefined;
  globalThis.Date.now = () => h.now;
  globalThis.setInterval = (fn, ms) => { h.intervals.push({ fn, ms }); return h.intervals.length; };
  globalThis.clearInterval = () => {};
  globalThis.setTimeout = (fn) => { return 0; };
  globalThis.clearTimeout = () => {};
  globalThis.AbortController = class { constructor() { this.signal = { aborted: false }; } abort() { this.signal.aborted = true; } };
  globalThis.fetch = async (path) => {
    if (path === "/api/events") return { ok: true, status: 200, body: { getReader: () => ({ read: () => new Promise(() => {}) }) } };
    return { ok: true, status: 200, json: async () => (path === "/api/sessions" ? sessions : {}) };
  };
  h.boot = async () => { await import(`${APP}?r=${++instance}`); await flush(); };
  // Opening Settings the way the operator does: clicking its tab.
  h.openSettings = async () => {
    const click = h.listeners.get("tabbar.tab:click");
    assert.ok(click, `no tab button listener — listeners: ${[...h.listeners.keys()]}`);
    els.get("tabbar.tab").dataset.tab = "settings";
    click();
    await flush();
  };
  h.fire = async (id, ev, e) => {
    const fn = h.listeners.get(`${id}:${ev}`);
    assert.ok(fn, `nothing listens for "${ev}" on #${id} — listeners: ${[...h.listeners.keys()]}`);
    h.writes.length = 0;
    fn(e);
    await flush();
  };
  return h;
}

// One session that every setting keeps, and one that each setting removes — or
// the board's HTML would be identical before and after, `paint` would skip the
// write, and the test would pass on a change that changed nothing.
const fleet = [
  { id: "a", name: "a", machine: "m", status: "working", last_seen_at: "2026-09-09T09:59:58Z", usage: {} },
  { id: "b", name: "b", machine: "m", status: "ended", last_seen_at: "2026-09-09T08:00:00Z", usage: {} },
];

test("turning ended sessions on repaints the board", async () => {
  const h = harness(fleet);
  await h.boot();
  await h.openSettings();
  await h.fire("ended-toggle", "change", { target: { checked: true } });
  assert.ok(h.writes.includes("tab-sessions"),
    `the board was not repainted after the setting changed; written: ${JSON.stringify(h.writes)}`);
});

// Its own fleet: the ended session is already hidden by default, so hiding by age
// has to act on a session that is *not* ended or the board does not change.
const quietFleet = [
  { id: "a", name: "a", machine: "m", status: "working", last_seen_at: "2026-09-09T09:59:58Z", usage: {} },
  { id: "c", name: "c", machine: "m", status: "idle", last_seen_at: "2026-09-09T08:00:00Z", usage: {} },
];

test("changing the idle threshold repaints the board", async () => {
  const h = harness(quietFleet);
  await h.boot();
  await h.openSettings();
  await h.fire("idle-select", "change", { target: { value: String(15 * 60 * 1000) } });
  assert.ok(h.writes.includes("tab-sessions"),
    `the board was not repainted after the setting changed; written: ${JSON.stringify(h.writes)}`);
});
