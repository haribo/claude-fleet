# ADR-0014: Retire remote-control detection

## Status

Accepted (#789).

Deprecates [design/remote-control.md](../design/remote-control.md), which is kept
rather than deleted: the decision it records was sound for the world it was
written in.

## Context

`/rc` lets a Claude Code session be driven from claude.ai or the Android app.
vigie never pilots anything ([ADR-0005](0005-observe-only.md)); it marked those
sessions on the board so an operator could see which ones someone might already
be driving from elsewhere, and offered the link that reopens them.

It could not observe `/rc` — that happens inside Claude Code — so it read one
field of the session file Claude Code writes, and `design/remote-control.md` § 2
stated the rule:

> A session is remotely controlled **iff its Claude session file has a non-empty
> `bridgeSessionId`**.

That was true when it was written. It is not any more. Measured on one machine, 9
live sessions, **none** under `/rc`:

- all 9 carry a non-empty `bridgeSessionId`;
- every other field that could separate them is constant — `entrypoint: "cli"`,
  `kind: "interactive"`, `peerProtocol: 1` across all 9;
- the only field that varies, `peerFeatures`, tracks the Claude Code **version**
  (absent on 2.1.231, `["notify_idle"]` on 2.1.236), not the session's state.

The column was not rendering wrongly. It reported, faithfully, a field that had
stopped distinguishing anything — and the daemon shipped `remote_control: true`
with a resume URL for sessions nobody was driving.

**What that measurement does not prove**, stated rather than glossed: there is no
positive example in it. It shows that nothing varies among nine sessions that are
*not* remote-controlled; it does not show that no field would light up if one
were.

## Decision

**Remote-control detection is removed** — the reading, the two fields it
travelled in, the column, the `rc` sort key and the `rc` filter token, in all
three clients.

It is not rebuilt on another field, and the decision is taken **without** running
the experiment that would find one. Even a field that distinguished today would
be another undocumented, unversioned key of Claude Code's private registry: the
same kind that just changed meaning underneath us, without breaking anything and
without any signal that it had.

This is [ADR-0012](0012-retire-the-stalled-status.md)'s argument on a second
subject. `stalled` claimed a verdict a timer could not support; `rc` claimed a
state a borrowed field no longer carried. **vigie must not display a distinction
it has no means of observing.**

### The database columns stay

`remote_control` and `remote_url` remain in the `sessions` table, written by no
one and read by no one. Migrations here are immutable once shipped, and a
migration that drops a column destroys data on every operator's machine to
reclaim two unused fields. They are dead columns, and that is cheaper than the
alternative.

## Consequences

- An operator who uses `/rc` loses the mark on those sessions and the one-click
  link that reopened them from the web. That is the price of not asserting what
  cannot be checked, and it is recoverable the day Claude Code exposes a field it
  documents.
- The `rc` filter token becomes an ordinary two-letter search. One shared fixture
  case survives to say so.
- No test would have caught the drift, and adding one would not help: a fixture
  pins the field's *assumed* meaning, so it passes more comfortably the further
  that meaning moves. What caught it was reading the real registry — which is the
  transferable lesson, and it applies to every field vigie reads from Claude Code
  without owning it: the registry status, the transcript's shapes, the
  task-notification format.

## Alternatives considered

**Rebuild on a field that still distinguishes.** Rejected above: it puts us back
here, silently, on the next change to a private format.

**Keep the column and mark it unreliable.** A column that means "possibly" is
worse than no column: the operator cannot act on it, and it still occupies the
width that #788 has just shown is scarce.

## References

- [design/remote-control.md](../design/remote-control.md) — deprecated by this ADR
- [ADR-0012](0012-retire-the-stalled-status.md) — the same argument, first use
- [ADR-0005](0005-observe-only.md) — why vigie never drove `/rc` in the first place
