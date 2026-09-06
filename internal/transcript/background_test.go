package transcript

import "testing"

// #748. A session waiting on a backgrounded command reported `idle`, so the board
// offered it as free and an operator scanning for one interrupted a session that
// was going to resume by itself.
//
// The pairing cannot hold it: Claude Code answers the launch at once — measured
// at 1.8–3.3 s across 1079 launches — so by the time the watcher looks, the
// `tool_use` is resolved and the turn reads finished. What holds it is the
// `<task-notification>`, the same close an async subagent gets.

const (
	bgLaunch = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_bg","name":"Bash","input":{"command":"gh pr checks 12 --watch","run_in_background":true}}]}}`
	// What Claude Code actually writes back, seconds later, while the command runs.
	bgAccepted = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_bg","content":"Command running in background with ID: bq7x1a2c3"}]}}`
	bgStop     = `{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"Started it in the background."}]}}`
)

func bgNotification(status string) string {
	return `{"type":"user","message":{"content":"<task-notification>\n<task-id>bq7x1a2c3</task-id>\n<tool-use-id>toolu_bg</tool-use-id>\n<status>` +
		status + `</status>\n<summary>Background command \"watch the checks\" ` + status + `</summary>\n</task-notification>"}}`
}

func TestABackgroundCommandKeepsTheSessionBusyAfterItsLaunchIsAnswered(t *testing.T) {
	info := parseLines(t, bgLaunch, bgAccepted, bgStop)
	if !info.BackgroundActive {
		t.Error("BackgroundActive = false while the command is still running — the session reads idle and the board offers it as free")
	}
	if info.PendingTool != "" {
		t.Errorf("PendingTool = %q, want empty — a background command is not what DETAIL names", info.PendingTool)
	}
}

// Every terminal status closes it. Only `completed` was recognized, and the
// corpus carries 87 `failed` and 13 `killed` against 836 `completed` — one
// command in ten would have stayed open for good.
func TestEveryTerminalNotificationClosesTheCommand(t *testing.T) {
	for _, status := range []string{"completed", "failed", "killed"} {
		info := parseLines(t, bgLaunch, bgAccepted, bgStop, bgNotification(status))
		if info.BackgroundActive {
			t.Errorf("%s: BackgroundActive still true — the session reads busy for the rest of its life", status)
		}
	}
}

// A notification that says the command is still going is not a close.
func TestARunningNotificationLeavesTheCommandOpen(t *testing.T) {
	info := parseLines(t, bgLaunch, bgAccepted, bgStop, bgNotification("running"))
	if !info.BackgroundActive {
		t.Error("a `running` notification closed the command")
	}
}

// The lost-notification case, and the only thing that closes it: the operator
// typing. There is no liveness cap — a command that runs for two days is a
// session working for two days (design: session-status.md § 2).
func TestTheOperatorsNextPromptClosesACommandThatNeverReported(t *testing.T) {
	prompt := `{"type":"user","message":{"content":"now do the other thing"}}`
	info := parseLines(t, bgLaunch, bgAccepted, bgStop, prompt)
	if info.BackgroundActive {
		t.Error("a prompt did not close a background command whose notification never came")
	}
}

// A notification is not a prompt, however much it looks like one: it must close
// the command it names and leave its siblings alone (#662).
func TestANotificationDoesNotRetireASiblingCommand(t *testing.T) {
	other := `{"type":"assistant","message":{"id":"m3","content":[{"type":"tool_use","id":"toolu_bg2","name":"Bash","input":{"command":"sleep 999","run_in_background":true}}]}}`
	otherAccepted := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_bg2","content":"Command running in background with ID: bzzz"}]}}`
	info := parseLines(t, bgLaunch, bgAccepted, other, otherAccepted, bgStop, bgNotification("completed"))
	if !info.BackgroundActive {
		t.Error("closing one background command retired the other")
	}
}
