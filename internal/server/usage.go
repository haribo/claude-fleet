package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

// usageLeaseTTL is longer than the fetch cadence so a missed cycle doesn't hand
// the lease around; a killed holder's lease expires after this.
const usageLeaseTTL = 12 * time.Minute

// usageMetaKey is where the latest usage snapshot is stored.
const usageMetaKey = "usage"

func (s *Server) handleUsageLease(w http.ResponseWriter, r *http.Request) {
	var req api.LeaseRequest
	if s.decodeBody(w, r, &req) != "" {
		return
	}
	if req.Holder == "" {
		s.writeError(w, http.StatusBadRequest, "holder is required")
		return
	}
	if req.Release {
		// The holder fetched nothing and is handing the lease back, so the next
		// machine can try. Without this one machine with no local credentials keeps
		// renewing and the gauges stay empty for the whole fleet (#646).
		if err := s.store.ReleaseLease(r.Context(), req.Holder); err != nil {
			s.log.Error("releasing lease", "error", err)
			s.writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		s.writeJSON(w, http.StatusOK, api.LeaseResponse{})
		return
	}
	acquired, expiry, err := s.store.AcquireLease(r.Context(), req.Holder, usageLeaseTTL, s.now())
	if err != nil {
		s.log.Error("acquiring lease", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.writeJSON(w, http.StatusOK, api.LeaseResponse{Acquired: acquired, ExpiresAt: expiry})
}

// handlePostUsage stores the fleet's usage snapshot. The lease exists so exactly
// one machine fetches (docs/design/usage.md); until #515 nothing checked it here,
// so any caller could overwrite the figure the whole fleet reads, with any value.
//
// A watcher that predates the Holder field is refused rather than trusted. The
// consequence is visible rather than silent: the snapshot ages, and the TUI's
// state modal reports its age (#494), so an operator mid-upgrade sees a stale
// figure instead of a wrong one.
func (s *Server) handlePostUsage(w http.ResponseWriter, r *http.Request) {
	var rep api.UsageReport
	if s.decodeBody(w, r, &rep) != "" {
		return
	}
	// Percentages, not arbitrary numbers: a negative or >100 figure renders as a
	// gauge that means nothing, and the client has no way to tell it apart from a
	// real one. Checked before the lease now: a malformed payload is the caller's
	// to fix whoever holds the lease, and refusing it costs nothing.
	if !isPercent(rep.FiveHourPct) || !isPercent(rep.SevenDayPct) {
		s.writeError(w, http.StatusBadRequest, "usage percentages must be between 0 and 100")
		return
	}
	blob, err := json.Marshal(rep)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// The lease is checked *by* the write, not before it. An empty holder is
	// refused by the same rule rather than by a check of its own: it holds no
	// lease, whoever it is. Read then write left a
	// gap a machine could lose its lease in, and the reading it had fetched
	// minutes earlier then landed on top of a fresher one from whoever had taken
	// over (#774).
	holder, written, err := s.store.SetMetaIfLeaseHolder(r.Context(), usageMetaKey, string(blob), rep.Holder, s.now())
	if err != nil {
		s.log.Error("storing usage", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !written {
		s.log.Warn("refusing usage from a machine that does not hold the lease",
			"from", rep.Holder, "holder", holder)
		s.writeError(w, http.StatusConflict, "the usage lease is held by another machine")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	v, ok, err := s.store.GetMeta(r.Context(), usageMetaKey)
	if err != nil {
		s.log.Error("reading usage", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var rep api.UsageReport
	if ok {
		if err := json.Unmarshal([]byte(v), &rep); err != nil {
			s.log.Error("decoding usage", "error", err)
		}
	}
	s.writeJSON(w, http.StatusOK, rep)
}

// isPercent reports whether v is a usable percentage. NaN and ±Inf fail it, as
// they must: they survive JSON round-trips through some encoders and render as
// nothing at all.
func isPercent(v float64) bool {
	return v >= 0 && v <= 100
}
