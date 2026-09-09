// #805. The browser refreshes its sessions from the live stream and from the
// 5-second tick, with nothing distinguishing one answer from another. Two in
// flight and the slower one lands last — so a snapshot can go backwards.
//
// Not only a stale board: notifications are decided from the snapshot just
// stored, by comparing it against a remembered set, so an answer that moves
// backwards can re-arm a session already announced and fire for it twice.
//
// The terminal numbers its fetches and drops any answer that is not the newest.
// It does that because it went wrong there once; this is the same guard in the
// second client.

import assert from "node:assert/strict";
import { test } from "node:test";

const APP = new URL("../../internal/web/static/app.js", import.meta.url);
let instance = 0;
const flush = async (n = 10) => { for (let i = 0; i < n; i++) await new Promise((r) => setImmediate(r)); };

function harness() {
  const h = { writes: [], listeners: new Map(), intervals: [], pending: [], now: Date.parse("2026-09-09T10:00:00Z") };
  const element = (id) => new Proxy(function () {}, {
    get: (_t, k) =>
      k === "addEventListener" ? (ev, fn) => h.listeners.set(`${id}:${ev}`, fn)
        : k === "querySelectorAll" ? () => []
          : k === "dataset" ? {}
            : k === "classList" || k === "style" ? element(id)
              : k === "hidden" || k === "value" || k === "innerHTML" || k === "textContent" ? ""
                : k === "children" || k === "options" ? []
                  : k === Symbol.toPrimitive || k === "toString" ? () => ""
                    : element(id),
    set: (_t, k, v) => { if (k === "innerHTML") h.writes.push({ id, html: v }); return true; },
    apply: () => element(id),
    has: () => true,
  });
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
  globalThis.Date.now = () => h.now;
  globalThis.setInterval = (fn, ms) => { h.intervals.push({ fn, ms }); return h.intervals.length; };
  globalThis.clearInterval = () => {};
  globalThis.setTimeout = () => 0;
  globalThis.clearTimeout = () => {};
  globalThis.AbortController = class { constructor() { this.signal = { aborted: false }; } abort() { this.signal.aborted = true; } };

  // /api/sessions answers only when a test says so, and in the order it chooses.
  globalThis.fetch = async (path) => {
    if (path === "/api/events") return { ok: true, status: 200, body: { getReader: () => ({ read: () => new Promise(() => {}) }) } };
    if (path !== "/api/sessions") return { ok: true, status: 200, json: async () => ({}) };
    return new Promise((resolve) => h.pending.push(resolve));
  };
  h.answer = async (i, sessions) => {
    h.pending[i]({ ok: true, status: 200, json: async () => sessions });
    await flush();
  };
  h.boot = async () => { await import(`${APP}?o=${++instance}`); await flush(); };
  h.tick = async () => { h.intervals.find((x) => x.ms === 5000).fn(); await flush(); };
  h.lastBoard = () => {
    const w = h.writes.filter((x) => x.id === "tab-sessions");
    return w.length ? w[w.length - 1].html : "";
  };
  return h;
}

const s = (name, status, seen) => ({ id: name, name, machine: "m", status, last_seen_at: seen, usage: {} });

test("a slow answer does not overwrite a newer one", async () => {
  const h = harness();
  await h.boot();
  await h.answer(0, [s("alpha", "idle", "2026-09-09T09:00:00Z")]); // the first load settles
  await h.tick();                        // fetch 1 — the periodic refresh
  await h.tick();                        // fetch 2 — the live stream, while 1 is still out

  const older = [s("alpha", "idle", "2026-09-09T09:00:00Z")];
  const newer = [s("alpha", "working", "2026-09-09T09:59:59Z")];

  await h.answer(2, newer);              // the later request comes back first
  await h.answer(1, older);              // the earlier one straggles in behind it

  const board = h.lastBoard();
  assert.match(board, /working/,
    "the board fell back to the older snapshot; a request that was already superseded won");
  assert.doesNotMatch(board, />idle</,
    `an answer older than one already applied reached the screen:\n${board}`);
});

// The ordinary case is untouched: an answer that is the newest is applied.
test("the newest answer is applied", async () => {
  const h = harness();
  await h.boot();
  await h.answer(0, [s("alpha", "waiting", "2026-09-09T09:59:59Z")]);
  assert.match(h.lastBoard(), /waiting/, "the only answer in flight was discarded");
});
