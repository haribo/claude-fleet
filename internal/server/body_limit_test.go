package server

import (
	"net/http"
	"strings"
	"testing"
)

// #740. Every request body is capped, and only the report endpoint said so. The
// other four decode the same way and answered `400 invalid json body` — telling
// an operator their JSON is malformed when it was fine and the body was simply
// too big. The two need different fixes, and the wrong one was named.

// oversized is a syntactically valid body past the cap, so the failure is the cap
// and not the parser.
func oversized() []byte {
	return []byte(`{"machine":"` + strings.Repeat("m", maxBodyBytes+1) + `"}`)
}

func TestAnOversizedBodyIsRefusedAsTooLargeEverywhere(t *testing.T) {
	posts := []string{
		"/api/report",
		"/api/settings",
		"/api/usage",
		"/api/usage/lease",
		"/api/watcher/heartbeat",
	}
	for _, path := range posts {
		srv := newTestServer(t)
		rec := do(t, srv, http.MethodPost, path, oversized(), true)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("POST %s with an oversized body = %d, want 413 — it reads as malformed JSON, which it is not",
				path, rec.Code)
		}
	}
}

// Malformed JSON is still a 400: the distinction only means something if both
// answers survive.
func TestMalformedJsonIsStillABadRequest(t *testing.T) {
	posts := []string{
		"/api/report",
		"/api/settings",
		"/api/usage",
		"/api/usage/lease",
		"/api/watcher/heartbeat",
	}
	for _, path := range posts {
		srv := newTestServer(t)
		rec := do(t, srv, http.MethodPost, path, []byte(`{not json`), true)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s with malformed json = %d, want 400", path, rec.Code)
		}
	}
}
