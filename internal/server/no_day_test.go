package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #768. A report with no timestamp had its token growth attributed to *today*,
// because the day was derived with a fallback to the current time rather than
// failing. `stats_daily` is never recomputed, so the wrong day stayed wrong.
//
// `rejectReport` accepts an empty timestamp on purpose — absent is not malformed,
// and it renders as a dash — while the comment above it lists this exact harm
// among the reasons a *bad* timestamp is refused. The reasoning admitted the harm
// and permitted the input that causes it.
//
// Nothing is lost by declining to count it: the mark #669 introduced means
// "already in stats_daily", so leaving it where it is makes the next report
// carrying an instant count the whole growth.

func dailyTotals(t *testing.T, srv *Server) (tokens int64, days []string) {
	t.Helper()
	rec := do(t, srv, http.MethodGet, "/api/stats", nil, true)
	var resp api.StatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Daily {
		tokens += d.OutputTokens
		days = append(days, d.Day)
	}
	return tokens, days
}

func TestGrowthWithNoDayIsNotAttributedToToday(t *testing.T) {
	srv := newTestServer(t)
	postReport(t, srv, `{"event":"UserPromptSubmit","session_id":"s1","machine":"m","model":"opus","usage":{"output_tokens":500}}`)

	tokens, days := dailyTotals(t, srv)
	if tokens != 0 {
		t.Errorf("%d output tokens landed on %v from a report naming no day; stats_daily is never recomputed, so that day stays wrong", tokens, days)
	}
}

// And it is not lost either: the next report that does carry an instant counts
// the whole growth, because the mark never moved.
func TestTheGrowthIsCountedOnTheNextReportThatNamesADay(t *testing.T) {
	srv := newTestServer(t)
	postReport(t, srv, `{"event":"UserPromptSubmit","session_id":"s1","machine":"m","model":"opus","usage":{"output_tokens":500}}`)
	postReport(t, srv, `{"event":"Stop","session_id":"s1","machine":"m","model":"opus","usage":{"output_tokens":800},"timestamp":"2026-09-07T10:00:00Z"}`)

	tokens, days := dailyTotals(t, srv)
	if tokens != 800 {
		t.Errorf("output_tokens = %d, want 800 — the growth skipped earlier has to be recovered, not dropped", tokens)
	}
	if len(days) != 1 || days[0] != "2026-09-07" {
		t.Errorf("days = %v, want only the day the report named", days)
	}
}

// A report that does name a day is untouched.
func TestAReportThatNamesADayStillCounts(t *testing.T) {
	srv := newTestServer(t)
	postReport(t, srv, `{"event":"UserPromptSubmit","session_id":"s1","machine":"m","model":"opus","usage":{"output_tokens":500},"timestamp":"2026-09-07T10:00:00Z"}`)

	tokens, days := dailyTotals(t, srv)
	if tokens != 500 || len(days) != 1 || days[0] != "2026-09-07" {
		t.Errorf("tokens = %d on %v, want 500 on 2026-09-07", tokens, days)
	}
}
