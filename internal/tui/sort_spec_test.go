package tui

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// #802. v0.14.0 removed a sort key and a filter token, and `session-list.md` went
// on teaching both — an `Accepted` specification describing a feature the binary
// no longer has. This repository's position is that the code is never the source
// of truth, which makes that the authority being wrong rather than a stale note.
//
// The docs suite did not catch it and would not have: it counts statuses,
// headings and routes. #718 produced a guard for the status vocabulary after
// `stalled` survived in the README; nothing covered the same accident on a
// feature.
//
// This is the narrow guard that does: the sort keys the specification lists are
// the sort keys that exist. It lives here rather than in test/docs because
// `sortNames` is what it has to compare against, and a guard that reads a copy
// instead of the thing is the drift it was written to catch.

// specSortKeys reads the keys from § 2's table — the first cell of each row,
// backticked — under the header naming it.
func specSortKeys(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../../docs/design/session-list.md") //nolint:gosec // our own committed spec
	if err != nil {
		t.Fatalf("reading the specification: %v", err)
	}
	body := string(b)
	i := strings.Index(body, "| Key ")
	if i < 0 {
		t.Fatal("the sort-key table is gone from session-list.md; if the sort keys stopped being specified, this test goes with them")
	}
	rest := body[i:]
	if end := strings.Index(rest, "\n\n"); end > 0 {
		rest = rest[:end]
	}
	row := regexp.MustCompile("(?m)^\\| `([^`]+)`")
	matches := row.FindAllStringSubmatch(rest, -1)
	keys := make([]string, 0, len(matches))
	for _, m := range matches {
		keys = append(keys, m[1])
	}
	if len(keys) == 0 {
		t.Fatalf("no keys read from the table:\n%s", rest)
	}
	return keys
}

func TestTheSpecificationListsTheSortKeysThatExist(t *testing.T) {
	spec := specSortKeys(t)
	have := make([]string, 0, len(sortNames))
	for _, n := range sortNames {
		have = append(have, n)
	}
	sort.Strings(spec)
	sort.Strings(have)

	known := map[string]bool{}
	for _, n := range have {
		known[n] = true
	}
	for _, k := range spec {
		if !known[k] {
			t.Errorf("the specification teaches the sort key %q, which the code does not implement — an operator is told about something that is not there", k)
		}
	}
	listed := map[string]bool{}
	for _, k := range spec {
		listed[k] = true
	}
	for _, n := range have {
		if !listed[n] {
			t.Errorf("the sort key %q exists and the specification does not list it — the table is what an operator reads to learn them", n)
		}
	}
}
