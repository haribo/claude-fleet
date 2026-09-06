package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/store"
)

// #759. Two secrets coexist the moment an operator adopts $VIGIE_TOKEN: the one
// they set, and the one the daemon generated for itself earlier and still holds.
// Taking the variable back out re-keys the fleet — every machine is refused, and
// the failure looks like a network or a certificate problem.
//
// `vigied token` already refuses to print the stored token in exactly this
// situation (#720). `serve` served on it and said nothing.

func continuityStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestARestartWithoutTheVariableSaysItReKeyedTheFleet(t *testing.T) {
	ctx := context.Background()
	st := continuityStore(t)
	if err := st.SetMeta(ctx, "token", "the-leftover"); err != nil {
		t.Fatal(err)
	}

	// A run with the variable set: it records which token is live and persists
	// nothing, leaving the earlier one in the store.
	t.Setenv(tokenEnv, "the-one-the-fleet-has")
	if _, _, err := resolveToken(ctx, st); err != nil {
		t.Fatal(err)
	}

	// The operator takes the variable out of the unit file and restarts.
	t.Setenv(tokenEnv, "")
	tok, warning, err := resolveToken(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "the-leftover" {
		t.Errorf("token = %q, want the stored one — serving is not the thing to change", tok)
	}
	if warning == "" {
		t.Fatal("the fleet was re-keyed and the daemon said nothing; the operator sees every machine refused and no reason")
	}
	for _, want := range []string{"VIGIE_TOKEN", "vigied token", "refused"} {
		if !strings.Contains(warning, want) {
			t.Errorf("the warning does not mention %q — it has to name the way out, not just the fact:\n%s", want, warning)
		}
	}
	if strings.Contains(warning, "the-leftover") || strings.Contains(warning, "the-one-the-fleet-has") {
		t.Error("the warning carries a token; it goes to the log, where the secret must not")
	}
}

// The ordinary restart is silent. A daemon that never saw the variable is not in
// this situation, and a warning it cannot act on is noise.
func TestAnOrdinaryRestartSaysNothing(t *testing.T) {
	ctx := context.Background()
	st := continuityStore(t)
	t.Setenv(tokenEnv, "")

	first, warning, err := resolveToken(ctx, st)
	if err != nil || warning != "" {
		t.Fatalf("first start: warning %q, err %v", warning, err)
	}
	again, warning, err := resolveToken(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Errorf("the generated token did not survive the restart: %q then %q", first, again)
	}
	if warning != "" {
		t.Errorf("an ordinary restart warned: %s", warning)
	}
}

// An empty stored value is not a token. Go's constant-time comparison reports two
// empty strings as equal, so serving on one authenticates any request whose header
// is exactly `Authorization: Bearer ` — the whole API to anyone who can reach the
// port. `vigied token` already refuses it.
func TestAnEmptyStoredTokenIsNotServed(t *testing.T) {
	ctx := context.Background()
	st := continuityStore(t)
	t.Setenv(tokenEnv, "")
	if err := st.SetMeta(ctx, "token", ""); err != nil {
		t.Fatal(err)
	}

	tok, _, err := resolveToken(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("the daemon would serve on an empty token: `Authorization: Bearer ` authenticates the whole API")
	}
	if len(tok) != 64 {
		t.Errorf("token = %q (len %d), want a freshly generated one", tok, len(tok))
	}
}
