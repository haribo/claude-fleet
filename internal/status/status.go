// Package status holds the one list of session statuses vigie can report.
//
// The vocabulary itself is specified in docs/design/session-status.md § 1; this
// package is its executable copy, and status_test.go fails if the two disagree.
// The design document stays the source of truth — the code is checked against it,
// never the other way round.
//
// It exists because the list used to be hand-copied into four places (the metrics
// gauge, the TUI sort, the web dashboard, the GNOME extension), each incomplete in
// a different way and none of them checked. Adding `compacting` (#342) reached two
// of the four, and nothing noticed for two releases (#421, #422, #423).
//
// Two of those copies are gone. Since ADR-0011 (#617) the daemon derives a
// session's rank and whether it needs the operator, and every client renders the
// answer — so `Order` and `Attention` have one consumer each, not three. The tests
// that used to read those arrays back out of the JavaScript sources went with
// them: there is nothing left for two copies to disagree about.
//
// What they used to guard is worth keeping in view, because it is what a new copy
// would cost again. #464: four statuses nobody had ranked produced a NaN
// comparator in the dashboard, which does not sort badly — it stops sorting.
// #466: the GNOME indicator disagreed with the TUI about when to interrupt the
// operator. #538: the dashboard had dropped `error` from its own attention list,
// so a session stuck on a 529 carried no mark at all.
//
// `All` is still read by the dashboard and the indicator for styling and
// grouping. Each keeps its own ordered copy — a module-private constant is
// reachable no other way — and the copies are pinned against
// `test/fixtures/status-vocabulary.json` from both sides rather than scraped out
// of the JavaScript, which is what #618 and #619 replaced.
package status

// All is every status a session can hold, in the order the design document lists
// them: roughly most-active first, ending with the two that mean "not running".
//
// Order matters to consumers that group or pre-declare series, so it is part of
// the contract rather than an accident of declaration.
var All = []string{
	"working",
	"thinking",
	"compacting",
	"waiting",
	"idle",
	"error",
	"stale",
	"ended",
}

// Known reports whether s is a status vigie can produce. A consumer that renders
// unknown statuses defensively (rather than dropping them) is doing the right
// thing; this is for the ones that must decide, such as styling.
func Known(s string) bool {
	for _, k := range All {
		if k == s {
			return true
		}
	}
	return false
}

// Order is every status ranked for the status sort, most active first, exactly as
// docs/design/session-list.md § 2.1 lists them.
//
// It is separate from All because the two answer different questions: All is what
// exists, Order is how it sorts. Keeping the sort partial was the defect in #464 —
// the four statuses nobody ranked fell to a default that put them below `ended` in
// the TUI, and produced a NaN comparator in the web dashboard, which stops sorting
// altogether rather than sorting badly.
var Order = []string{
	"working",
	"thinking",
	"compacting",
	"waiting",
	"idle",
	"error",
	"stale",
	"ended",
}

// Rank is a status's position in the sort, lower first. An unknown status sorts
// last rather than first: a status this build has never heard of is the one thing
// we can say least about, so it does not get to head the table.
func Rank(s string) int {
	for i, k := range Order {
		if k == s {
			return i
		}
	}
	return len(Order)
}

// Attention are the statuses that call for the operator: the session is blocked
// and needs a human — it is waiting on input, or it errored.
//
// `stalled` used to be a third. It claimed a turn was parked on a hung tool, and
// vigie had no grounds for the claim: the tool pairing proves a call is
// outstanding, never that it is hung, and the verdict came from a timer over a
// duration only the operator can interpret. A session running an hour-long test
// suite was announced as a fault. Removed by
// [ADR-0012](../../docs/adr/0012-retire-the-stalled-status.md).
//
// A raised call is not here because it is not a status: it rides alongside one
// (ADR-0010). Anything deciding whether to interrupt the operator has to consider
// both, which is why this is a list to consult rather than a rule to reimplement.
var Attention = []string{"waiting", "error"}

// NeedsAttention reports whether a status is one the operator should be told
// about.
func NeedsAttention(s string) bool {
	for _, k := range Attention {
		if k == s {
			return true
		}
	}
	return false
}

// Reasons is why a session can be calling the operator, a closed vocabulary of
// codes ordered by precedence. It is what a client renders a sentence from — the
// notification body used to be the raw status, so a session stopped on a 529 was
// announced as `is waiting`, which is neither what happened nor what to do about
// it (#742).
//
// Codes, never prose. The daemon decides *which* reason applies, the same way it
// decides the attention set (ADR-0011, #617) — a client that reimplements that
// gets it wrong eventually, and one already did (#538). How to say it is the
// client's, because the three surfaces are not alike: a GNOME toast names the
// machine and the branch because it carries no other context, where the TUI has
// the board on screen. Sending the sentence instead would flatten a difference
// that is correct, and fix the language and register for every client at once —
// which is why [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html) provides a
// human-readable field and tells consumers not to act on it.
//
// Pinned against test/fixtures/attention-reasons.json from both sides.
var Reasons = []string{"call", "waiting", "error"}

// Reason returns why a session is calling the operator, or "" when nothing is.
//
// A raised call outranks the status: the session said so itself, where `waiting`
// and `error` are deductions about it (ADR-0010). That is the same precedence the
// jump-to-next key already applies.
func Reason(s string, hasCall bool) string {
	if hasCall {
		return "call"
	}
	if NeedsAttention(s) {
		return s
	}
	return ""
}
