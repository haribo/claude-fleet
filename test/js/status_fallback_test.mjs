// #766. `lib.js` said two opposite things about an unknown status, eleven lines
// apart: the note above the vocabulary said it falls back to `idle`, and
// `statusClass` right below said it falls back to a neutral class and is never
// relabelled. The second describes the code (#719); the first was what #719
// removed, and it is the one a reader meets first.
//
// A comment cannot be tested. The behaviour it described can, so that a future
// note claiming the old fallback is contradicted by something that fails.

import assert from "node:assert/strict";
import { test } from "node:test";

import { statusClass, STATUSES } from "../../internal/web/static/lib.js";

test("an unknown status is never relabelled as another one", () => {
  const got = statusClass("stalled"); // retired by ADR-0012; stored rows still carry it
  assert.notEqual(got, "idle", "a status this build has retired is shown resting");
  assert.ok(!STATUSES.includes(got),
    `an unknown status was mapped onto ${got}, which is a claim about the session this build cannot make`);
});

test("a status it does know is passed through untouched", () => {
  for (const s of STATUSES) assert.equal(statusClass(s), s);
});
