package tui

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/apiclient"
	"github.com/haribo/claude-vigie/internal/config"
)

// subscribeEvents streams SSE notifications from the server, sending on out for
// each event and reporting connection state on conn (true = streaming, false =
// disconnected). It reconnects on failure; the tui's poll covers any gap.
func subscribeEvents(cfg *config.Config, out chan<- struct{}, conn chan<- bool) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/events"
	client := streamClient()
	for {
		streamEvents(client, url, cfg.Token, out, conn)
		sendState(conn, false) // disconnected; retry after a brief pause
		time.Sleep(2 * time.Second)
	}
}

// silenceLimit is how long the client waits to hear anything at all before it
// treats the stream as dead. The server sends a keep-alive comment every 10 s
// (internal/server/events.go), so this is three missed beats.
//
// It exists because a suspended machine's connection dies without a FIN or an
// RST: the socket stays open as far as the process is concerned, and a read on it
// blocks until the OS gives up on its keepalive probes — minutes. The reconnect
// loop below was correct all along and simply never ran, because the function it
// guards had not returned (#457).
var silenceLimit = 30 * time.Second

func streamEvents(client *http.Client, url, token string, out chan<- struct{}, conn chan<- bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A watchdog, reset by every line the stream delivers. Canceling the request
	// unblocks the read below, which is the only thing that can: a Scanner has no
	// deadline of its own.
	watchdog := time.AfterFunc(silenceLimit, cancel)
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	sendState(conn, true) // connected and streaming
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		watchdog.Reset(silenceLimit) // any line, comment included, proves it is alive
		if strings.HasPrefix(sc.Text(), "data:") {
			select {
			case out <- struct{}{}:
			default:
			}
		}
	}
}

// sendState reports a connection-state change without blocking; the buffered
// channel keeps the latest state the model has not yet read.
func sendState(conn chan<- bool, live bool) {
	select {
	case conn <- live:
	default:
	}
}

// streamClient is the event stream's own client: no request timeout, because the
// stream is long-lived by design and a deadline would cut a healthy one — and the
// same dead-connection tuning every other call the terminal makes already gets.
//
// It was a bare client. The health check exists because an HTTP/2 connection can
// survive a suspend open and mute, leaving the caller waiting about fifteen
// minutes for the kernel to give up (#732) — and the held-open stream is the
// connection most exposed to exactly that, while being the one not covered
// (#793).
//
// What it cost was bounded, which is why it went unnoticed: the terminal polls
// every 5 s regardless, so a dead stream loses the immediacy and not the board.
func streamClient() *http.Client {
	return &http.Client{
		Transport: apiclient.TuneForDeadConnections(http.DefaultTransport.(*http.Transport).Clone()),
	}
}
