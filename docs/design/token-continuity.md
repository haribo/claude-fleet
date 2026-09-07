# Token continuity — Design Specification

**Status:** Accepted (#759).

Source of truth for **which token the daemon serves on, and what it says when the
evidence is ambiguous**. One rule and one message; every decision here is about
which of the two secrets in play is the live one, and about who finds out.

---

## 1. The situation

A daemon takes its token from `$VIGIE_TOKEN` if it is set, and from the store
otherwise, generating and persisting one the first time it finds neither. A
daemon given the variable **persists nothing** — deliberately, so a secret handed
in through the environment does not end up written to a file.

Two secrets therefore coexist the moment an operator adopts the variable: the one
they set, in use, and the one the daemon generated for itself on some earlier
run, still in the store and no longer used by anything.

`vigied token` already had to tell them apart, because it prints one. It reads a
**fingerprint** — a hash of the token the daemon last started with, rewritten on
every start — and refuses to print a stored token the fingerprint contradicts:
handing that one over gets the machine refused, with nothing to suggest the
answer was wrong rather than the setup (#720).

## 2. The decision

**A daemon serves on the token it resolves, and says so out loud when the
fingerprint contradicts it.**

Concretely, with no variable set and a stored token whose fingerprint does not
match: the daemon serves on the stored token — the behaviour is unchanged — and
logs a warning at startup naming what happened and the two ways out.

The variable's removal re-keys the fleet. That is the fact; the defect was that
nothing said so. An operator who takes `VIGIE_TOKEN` out of a unit file gets a
daemon that comes back on a different secret and refuses every machine they own,
and the failure looks like a network or a certificate problem — the one shape of
failure that costs an afternoon.

### Why not refuse to start

Considered, and it is the reading `vigied token` takes. Refused because the two
commands answer different questions. `token` is asked "what should I hand my
machines?", and a confident wrong answer there is worse than none. `serve` is
asked to run, and a daemon that will not start takes the whole board down —
including the sessions that were reporting fine — to protect against a
misconfiguration the operator may have intended. The asymmetry is the point: the
cost of being wrong is not symmetric, so the answers are not either.

### Why not mint a fresh token

It invents a third secret nobody holds, turning a fleet that is refused into a
fleet that is refused **and** whose stored token no longer matches what anyone
was given. Strictly worse than either alternative.

## 3. The message

It names the situation, not the mechanism — the operator is not holding a copy of
this document:

```
the stored token is not the one this daemon last ran with; serving on the stored
token, so machines configured with the previous one will be refused. Set
VIGIE_TOKEN again to keep it, or run `vigied token` to read the one now in use.
```

`vigied token` is safe to point at here: once this start has rewritten the
fingerprint, the stored token *is* the live one, and the command prints it.

## 4. An empty stored token is not a token

A stored value of `""` is treated as absent, and a fresh token is generated.

Go's constant-time comparison reports two empty strings as equal, so a daemon
serving on an empty token authenticates any request whose header is exactly
`Authorization: Bearer ` — the whole API, read and write, to anyone who can
reach the port. `vigied token` already refuses an empty stored value; nothing
should be able to reach this state, and the cost of saying so is one condition.
