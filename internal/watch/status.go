// Deriving a session's status from what the registry and the transcript say,
// and the refinements layered on top of it (docs/design/session-status.md).
package watch

import (
	"time"

	"github.com/haribo/claude-vigie/internal/compaction"
	"github.com/haribo/claude-vigie/internal/presence"
	"github.com/haribo/claude-vigie/internal/transcript"
)

// resolveStatus derives a session's status, activity, and report time. When
// Claude Code's session registry (#254) covers the session, its own status is
// authoritative — busy→working, idle/shell→idle, waiting→waiting — and a
// confidently-dead process reads ended. This is exactly what a hooks-free machine
// needs: without it, a quiet-but-alive session with no hook mapping is wrongly
// derived as ended. The SEEN timestamp deliberately stays on the fresh transcript
// activity, not the registry (whose status time lags a running turn). error and
// thinking still refine the base. Sessions the registry does not cover (older
// clients) fall back to the transcript heuristic.
func resolveStatus(reg map[string]sessionRecord, regByProc map[procID]string, id string, info *transcript.Info, activityAge time.Duration, lastActivity, now time.Time) (status, activity string, reportAt time.Time) {
	activity, reportAt = info.Activity, lastActivity
	rec, known := reg[id]
	var base string
	switch {
	case known && registryDead(rec):
		return "ended", activity, reportAt // the backing process is gone
	case known:
		base = withError(mapRegistryStatus(rec.Status), info.LastAPIError)
		switch {
		case rec.Status == "shell" && base == "idle" && info.PendingTool != "":
			// `shell` names two situations Claude Code does not distinguish: the
			// operator dropped to a shell prompt — alive, producing nothing, a real
			// rest (#280) — and a Bash tool executing, which is work in progress.
			// Measured on a live session, the registry sat at `shell` for 78 s of a
			// two-minute window while a foreground command ran (#661).
			//
			// The transcript separates them: an unanswered `tool_use` means Claude is
			// waiting on a command. Reading that as `idle` reported a session doing
			// nothing while its build ran — and since `idle` is the base the tool
			// pairing acts on, the same build was reported `stalled` after 45 s, the
			// false positive session-status.md § 2 says the five-minute window exists
			// to prevent.
			//
			// DETAIL keeps the transcript's own message: which tool is running is more
			// use to the operator than the word `shell`.
			//
			// `base == "idle"` guards a live API error, which withError has already
			// established and which outranks this.
			base = "working"
		case rec.Status == "shell":
			activity = "shell" // dropped to a shell: status stays idle, DETAIL says so (#280)
		case base == "waiting" && activity == "" && rec.WaitingFor != "":
			activity = capText(rec.WaitingFor, 80) // surface the ask in DETAIL
		}
	case superseded(id, regByProc):
		return "ended", activity, reportAt // switched in place on the same process (e.g. /clear) (#367)
	default:
		base = sessionStatus(id, info.LastStopReason, info.LastAPIError, activityAge)
	}
	base, activity = refineStatus(base, activity, id, info, activityAge, now)
	return base, activity, reportAt
}

// superseded reports whether a session that has left the registry was replaced
// *in place* on the same process — the `/clear` case: the old id's transcript is
// still fresh and its (reused) process alive, but that process now runs a
// different session id. Read-only: it reads the presence mapping vigie already
// keeps (ADR-0005). Without it the old transcript lingers as a ghost idle row,
// because the reused process reads alive (#367).
func superseded(id string, regByProc map[procID]string) bool {
	m, ok, err := presence.Load(id)
	if err != nil || !ok || m.PID <= 0 || m.StartTime == 0 {
		return false
	}
	cur, live := regByProc[procID{pid: m.PID, procStart: m.StartTime}]
	return live && cur != id
}

// refineStatus applies the transcript-derived refinements on top of the registry
// status and picks the matching "doing" message: thinking, then compacting
// (#342), then the tool-based background/subagent rules (#256/#344).
func refineStatus(base, activity, id string, info *transcript.Info, activityAge time.Duration, now time.Time) (string, string) {
	// A turn the operator killed is not a turn still reasoning. Both signals
	// survive an interrupt — the last block Claude produced was a thinking block,
	// and nothing clears that but a new assistant line, which a killed turn never
	// writes. Refining first would make the session `thinking` for good, and the
	// `interrupted` marker only ever attaches to a resting session, so it would
	// never be seen (#721).
	base = withThinking(base, info.Thinking && !info.Interrupted)
	base = withCompacting(base, compactingNow(id, info, now)) // opaque `working` during compaction → `compacting`
	prevBase := base
	base = refineWithTools(base, info, activityAge) // an outstanding tool call keeps the session working
	switch {
	case base == "compacting":
		activity = "compacting context"
	case base == "idle" && info.Interrupted:
		activity = "interrupted" // the operator killed the turn; still idle (#351)
	case base == "working" && activity == "" && info.PendingTool != "":
		activity = "running " + info.PendingTool
	case prevBase == "idle" && base == "working" && info.AgentsActive > 0 && activityAge < agentWindow:
		activity = info.AgentActivity // the work is running in a subagent
	case prevBase == "idle" && base == "working" && info.BackgroundActive:
		// The registry says `shell` for both an operator at a `!` prompt and a
		// session that launched a background command, and the #280 branch has
		// already written `shell` into DETAIL on that reading. Only the transcript
		// separates them, and it just did — so the message follows the status
		// rather than contradicting it (#748).
		activity = "background command"
	}
	return base, activity
}

// agentWindow bounds how long an in-flight subagent keeps its parent working
// without any parent-transcript activity. It is the liveness cap: past it, a
// close that never arrived (an undocumented <task-notification> format that
// drifted) self-heals instead of pinning the session to working forever. Well
// clear of the observed subagent runtimes (p90 ~8 min, max ~17 min) (#344).
const agentWindow = 30 * time.Minute

// refineWithTools reclassifies a quiet/idle session using the transcript's
// unresolved tool calls (#256). A tool call with no result is a session waiting on
// a command — foreground or backgrounded, a build or a subagent — and that is
// `working`. Only an idle base is touched, so working/waiting/error/ended are
// never overridden.
//
// It used to return `stalled` once a foreground call had been outstanding past a
// threshold. That claimed the turn was parked on a hung tool, which vigie has no
// grounds for: the pairing proves a call is outstanding, never that it is hung,
// and the verdict came from a timer over a duration only the operator can
// interpret. How long the call has been outstanding is on the row — SEEN counts
// from the `tool_use` line, and DETAIL names the tool
// ([ADR-0012](../../docs/adr/0012-retire-the-stalled-status.md)).
func refineWithTools(base string, info *transcript.Info, activityAge time.Duration) string {
	if base != "idle" {
		return base
	}
	if info.BackgroundActive {
		return "working" // a backgrounded tool is still running
	}
	if info.AgentsActive > 0 && activityAge < agentWindow {
		return "working" // an async subagent is still running (#344)
	}
	if info.PendingTool != "" {
		return "working" // Claude is waiting on a command
	}
	return base
}

// sessionStatus layers a transient "error" status on top of the base
// derivation: when the last assistant line was an API error (500/529/429…), a
// live session — one that would otherwise read working or idle — reports error
// until a later non-error line clears it. A closed session (ended) is never
// shown as error, so a stale transcript does not stay red forever.
func sessionStatus(sessionID, lastStopReason string, lastAPIError int, age time.Duration) string {
	return withError(statusFor(sessionID, lastStopReason, age), lastAPIError)
}

// withError layers a transient "error" over a live base status when the last
// assistant line was an API error: only working/idle become error (a closed or
// waiting session is never overwritten red).
func withError(base string, lastAPIError int) string {
	if lastAPIError != 0 && (base == "working" || base == "idle") {
		return "error"
	}
	return base
}

// compactWindow bounds how long a compaction marker keeps a session `compacting`
// without the transcript's closing boundary. It is the safety cap so an
// interrupted compaction never sticks — well past the observed 87–168 s range
// (#342, ADR-0008).
const compactWindow = 5 * time.Minute

// withCompacting refines an active status to "compacting" while the session is
// summarizing its context. Like withThinking it only touches a live turn
// (working/thinking); it never overrides waiting/error/ended/idle (#342).
func withCompacting(status string, compacting bool) string {
	if compacting && (status == "working" || status == "thinking") {
		return "compacting"
	}
	return status
}

// compactingNow reports whether a session is mid-compaction: a PreCompact marker
// exists, no transcript boundary has closed it, and it has not aged past
// compactWindow. It sweeps a resolved or expired marker so the state self-heals.
func compactingNow(id string, info *transcript.Info, now time.Time) bool {
	m, ok, err := compaction.Load(id)
	if err != nil || !ok {
		return false
	}
	started, ok := m.StartedAt()
	if !ok || boundaryCloses(info.LastCompactBoundary, started) || now.Sub(started) >= compactWindow {
		_ = compaction.Remove(id) // resolved, expired, or unparseable → sweep
		return false
	}
	return true
}

// boundaryCloses reports whether a compact_boundary at RFC3339 `boundary` closes
// a compaction that began at `started` (boundary at or after the start).
func boundaryCloses(boundary string, started time.Time) bool {
	t, err := time.Parse(time.RFC3339, boundary)
	return err == nil && !t.Before(started)
}

// withThinking refines an active status to "thinking" when the transcript's last
// assistant block is a thinking block — Claude is reasoning inside the turn. It
// only refines working/idle (a live turn); error, ended, and waiting are left as
// is. Heuristic: at rest a completed turn's last block is text/tool, so this is
// true only mid-turn (a turn aborted right after thinking may briefly mis-show it
// until the next scan).
func withThinking(status string, thinking bool) string {
	if thinking && (status == "working" || status == "idle") {
		return "thinking"
	}
	return status
}

// statusFor derives a session's status from process presence and transcript
// activity:
//   - mapping present & dead → ended (reliable even on a hard kill)
//   - transcript actively changing → working (mapping or not)
//   - mapping present & alive but idle → idle (for any duration)
//   - no mapping & inactive → ended (presumed closed; a live session gets a
//     mapping via the SessionStart/UserPromptSubmit backfill)
func statusFor(sessionID, lastStopReason string, age time.Duration) string {
	m, ok, err := presence.Load(sessionID)
	hasMapping := err == nil && ok
	switch {
	case hasMapping && presence.Status(m) == presence.Gone:
		return "ended" // Gone, never merely unreadable (#663)
	case activelyWorking(lastStopReason, age):
		return "working"
	case hasMapping:
		return "idle"
	default:
		return "ended"
	}
}

// activelyWorking reports whether the transcript shows work in progress: it
// changed within activeWindow, or the last turn stopped on a tool call still
// within toolWindow (a long-running tool that has not written yet).
func activelyWorking(lastStopReason string, age time.Duration) bool {
	return age < activeWindow || (lastStopReason == "tool_use" && age < toolWindow)
}
