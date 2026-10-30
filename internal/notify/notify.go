// Package notify works out what changed between two snapshots. It is pure:
// delivering the notes is the caller's job.
package notify

import "github.com/knightmare2600/vogter/internal/model"

// maxPerPoll caps new and resolved problems announced per poll, to avoid a
// flood after an outage.
const maxPerPoll = 3

// Kind is a note's type.
type Kind int

// Note kinds.
const (
	NewProblem Kind = iota
	Resolved
	HostDown
	HostUp
)

// Note is one announcement.
type Note struct {
	Kind     Kind
	Severity model.Severity
	Title    string
	Body     string
	Urgent   bool
}

// Diff compares cur with prev. seen holds the event ids announced or known
// so far, and the returned set replaces it. With no prev, nothing is
// announced: what was already there at start-up is not news.
func Diff(prev, cur *model.Snapshot, seen map[string]bool, minSev model.Severity) ([]Note, map[string]bool) {
	next := make(map[string]bool, len(cur.Problems))
	for i := range cur.Problems {
		next[cur.Problems[i].EventID] = true
	}
	if prev == nil {
		return nil, next
	}
	notes := []Note{}
	n := 0
	for i := range cur.Problems {
		p := &cur.Problems[i]
		if n == maxPerPoll {
			break
		}
		if seen[p.EventID] || p.Suppressed || p.Severity < minSev {
			continue
		}
		notes = append(notes, Note{NewProblem, p.Severity, p.Severity.Label() + " · " + p.Host, p.Name, p.Severity >= model.High})
		n++
	}
	// A full page proves nothing about the problems that fell off it
	// (zabterm announced those as resolved, D10).
	if !cur.Truncated && !prev.Truncated {
		n = 0
		for i := range prev.Problems {
			p := &prev.Problems[i]
			if n == maxPerPoll {
				break
			}
			if next[p.EventID] || p.Suppressed || p.Severity < minSev {
				continue
			}
			notes = append(notes, Note{Resolved, p.Severity, "Resolved · " + p.Host, p.Name, false})
			n++
		}
	}
	before := make(map[string]model.Availability, len(prev.Hosts))
	for i := range prev.Hosts {
		before[prev.Hosts[i].ID] = prev.Hosts[i].Availability
	}
	for i := range cur.Hosts {
		h := &cur.Hosts[i]
		was, ok := before[h.ID]
		if !ok {
			continue
		}
		switch {
		case was == model.Up && h.Availability == model.Down:
			why := h.Error
			if why == "" {
				why = h.Address
			}
			notes = append(notes, Note{Kind: HostDown, Title: "Unreachable · " + h.Name, Body: why, Urgent: true})
		case was == model.Down && h.Availability == model.Up:
			notes = append(notes, Note{Kind: HostUp, Title: "Back online · " + h.Name, Body: h.Address})
		}
	}
	return notes, next
}
