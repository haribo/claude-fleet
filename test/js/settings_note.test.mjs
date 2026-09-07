// #767. The Settings tab explained a read-only control with the wrong principle:
// "claude-vigie is observe-only". ADR-0005 and ADR-0007 are about never acting on
// a *session* — no command written into one, no operator state held about one.
// The session-retention window is a daemon setting, and the terminal client
// writes it, so the sentence cited a rule the product does not follow, in the one
// place an operator goes looking for the setting.
//
// The dashboard being a reader is a real decision — the README states it twice —
// and that is the reason it now gives.

import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";

const app = await readFile(new URL("../../internal/web/static/app.js", import.meta.url), "utf8");

test("the settings note does not borrow the observe-only principle", () => {
  const note = app.match(/<div class="set-note">.*?<\/div>/s);
  assert.ok(note, "the settings note is gone; if that is deliberate, this test goes with it");
  assert.doesNotMatch(note[0], /observe-only/,
    "the note explains a read-only setting with a principle that is about never acting on a session");
});

test("it still says where the setting can be changed", () => {
  const note = app.match(/<div class="set-note">.*?<\/div>/s)[0];
  assert.match(note, /read-only/, "the note no longer says the settings cannot be changed here");
  assert.match(note, /vigie/, "the note no longer says where they can be");
});
