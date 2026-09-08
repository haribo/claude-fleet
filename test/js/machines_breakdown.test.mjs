// #791. A machine's card shows how many sessions it runs and a row of pills
// breaking that down. The total counted every session; the pills drew only the
// statuses this build knows — so a card read `7 sessions` above pills adding up
// to six, with nothing saying where the seventh went.
//
// Two ways in, both ordinary: a row still stored under a status ADR-0012 retired
// and expected to survive in old rows, and a status the daemon starts sending
// before the browser bundle knows it — the usual order of a deploy.
//
// The terminal has done this since #509, and its comment states the rule: no
// session counted in one place and shown in none. The dashboard already degrades
// an unknown status rather than relabelling it in the session table (#719); this
// is the same treatment on the third surface.

import assert from "node:assert/strict";
import { test } from "node:test";

import { statusBreakdown, STATUSES } from "../../internal/web/static/lib.js";

test("every session is drawn somewhere, whatever its status", () => {
  const counts = { working: 3, idle: 3, stalled: 1 }; // `stalled` left the vocabulary with ADR-0012
  const total = Object.values(counts).reduce((a, b) => a + b, 0);
  const drawn = statusBreakdown(counts).reduce((a, r) => a + r.n, 0);
  assert.equal(drawn, total,
    "the card counts a session it draws in no pill; the two numbers disagree and nothing says why");
});

test("an unknown status keeps its own name and takes a neutral colour", () => {
  const [row] = statusBreakdown({ stalled: 2 });
  assert.equal(row.status, "stalled", "the status was relabelled; the operator cannot tell what it is");
  assert.equal(row.cls, "unknown", "an unknown status borrowed another status's colour");
});

test("the known statuses keep the vocabulary's order, and the unknown ones come last", () => {
  const got = statusBreakdown({ zulu: 1, idle: 1, working: 1, alpha: 1 }).map((r) => r.status);
  assert.deepEqual(got, ["working", "idle", "alpha", "zulu"],
    "order is part of the contract: most-active first, then what this build cannot place");
});

test("a status with no sessions is not drawn", () => {
  const got = statusBreakdown({ working: 2, idle: 0 }).map((r) => r.status);
  assert.deepEqual(got, ["working"], "an empty bucket took space on the card");
});

test("a fleet that fits the vocabulary looks exactly as it did", () => {
  const counts = Object.fromEntries(STATUSES.map((s, i) => [s, i + 1]));
  const got = statusBreakdown(counts);
  assert.deepEqual(got.map((r) => r.status), STATUSES);
  assert.ok(got.every((r) => r.cls === r.status), "a known status stopped being drawn as itself");
});
