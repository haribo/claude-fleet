// #742. The notification body used to be the raw status, so a session stopped on
// a 529 was announced as `error` in the browser and `is waiting` in the terminal
// — a word from the machine's vocabulary, and the wrong instruction.
//
// The daemon says which reason applies; each client writes its own sentence. What
// is checked here is that neither client can be handed a reason it has nothing to
// say about, and that the two do not quietly converge on identical wording when
// their surfaces are not alike.

import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";

import { bodyFor } from "../../internal/web/static/lib.js";
import { attentionReason } from "../../gnome-extension/lib.js";

const fixture = async (name) =>
  JSON.parse(await readFile(new URL(`../fixtures/${name}`, import.meta.url), "utf8"));

test("every reason the daemon can send has a sentence in both clients", async () => {
  const { reasons } = await fixture("attention-reasons.json");
  assert.ok(reasons.length > 0, "the fixture lists no reasons");

  for (const { reason, why } of reasons) {
    const s = { attention_reason: reason, machine: "laptop", git_branch: "main" };

    const web = bodyFor(s);
    assert.notEqual(web, "needs you",
      `the dashboard falls back to a generic body for ${reason} — ${why}`);
    assert.ok(web.length > 0, `the dashboard says nothing for ${reason}`);

    const gnome = attentionReason(s);
    assert.ok(gnome.length > 0, `the GNOME indicator says nothing for ${reason} — ${why}`);
  }
});

test("a session asking nothing gets no notification body from the indicator", () => {
  assert.equal(attentionReason({ attention_reason: "" }), "");
  assert.equal(attentionReason(null), "");
});

test("a raised call speaks for itself, and falls back when it carries no message", () => {
  const withMessage = { attention_reason: "call", call_message: "the migration needs a decision" };
  assert.equal(bodyFor(withMessage), "the migration needs a decision");
  assert.equal(attentionReason(withMessage), "the migration needs a decision");

  const bare = { attention_reason: "call" };
  assert.ok(bodyFor(bare).length > 0);
  assert.ok(attentionReason(bare).length > 0);
});

// The reason is the daemon's verdict, not something a client re-derives from the
// status. A session whose status says `waiting` but which the daemon did not mark
// must not be announced — that re-derivation is what #538 was.
test("neither client re-derives the reason from the raw status", () => {
  const notMarked = { status: "waiting", attention_reason: "" };
  assert.equal(attentionReason(notMarked), "");
  assert.equal(bodyFor(notMarked), "needs you");
});
