package tui

import (
	"net/http"
	"testing"
)

// #793. The terminal's event stream ran on a bare client while every other call
// it makes goes through the tuned one. That helper exists because of #732: after
// a suspend an HTTP/2 connection can stay open and answer nothing, and without
// health-check pings the caller waits on it for about fifteen minutes — observed
// three times on a watcher.
//
// The stream is the connection most exposed to a suspend, being the one held
// open, and it was the one not covered. The cost was bounded — the terminal polls
// every 5 s regardless, so the board stays fresh and what a dead stream loses is
// the immediacy — which is why this is a hardening rather than a reported fault.
//
// Asserted on the transport rather than against a network: what went wrong was a
// client built without the tuning, and that is what this catches. The behavior
// itself is covered where it can be reproduced, in
// internal/apiclient/deadconn_test.go.
func TestTheEventStreamDetectsADeadConnection(t *testing.T) {
	tr, ok := streamClient().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("the stream client's transport is %T, not one that can carry the HTTP/2 health check", streamClient().Transport)
	}
	if tr.HTTP2 == nil || tr.HTTP2.SendPingTimeout == 0 || tr.HTTP2.PingTimeout == 0 {
		t.Error("the event stream would wait on a connection that has stopped answering until the kernel gives up")
	}
}

// And it keeps no request timeout: the stream is long-lived by design, and a
// deadline would cut a healthy one.
func TestTheEventStreamHasNoRequestTimeout(t *testing.T) {
	if d := streamClient().Timeout; d != 0 {
		t.Errorf("stream client timeout = %s, want none — it would cut a working stream", d)
	}
}
