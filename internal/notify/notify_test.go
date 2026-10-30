package notify

import (
	"fmt"
	"testing"

	"github.com/knightmare2600/vogter/internal/model"
)

func prob(id string, s model.Severity) model.Problem {
	return model.Problem{EventID: id, Name: "p" + id, Host: "web", Severity: s}
}

func snap(ps ...model.Problem) *model.Snapshot { return &model.Snapshot{Problems: ps} }

func TestFirstSnapshotOnlySeeds(t *testing.T) {
	notes, seen := Diff(nil, snap(prob("1", model.Disaster)), nil, model.Average)
	if len(notes) != 0 || !seen["1"] {
		t.Errorf("%+v %v", notes, seen)
	}
}

func TestNewAndResolved(t *testing.T) {
	prev := snap(prob("1", model.High))
	_, seen := Diff(nil, prev, nil, model.Average)
	cur := snap(prob("2", model.Disaster))
	notes, seen := Diff(prev, cur, seen, model.Average)
	if len(notes) != 2 {
		t.Fatalf("%+v", notes)
	}
	if n := notes[0]; n.Kind != NewProblem || n.Title != "Disaster · web" || n.Body != "p2" || !n.Urgent {
		t.Errorf("%+v", n)
	}
	if n := notes[1]; n.Kind != Resolved || n.Title != "Resolved · web" || n.Urgent {
		t.Errorf("%+v", n)
	}
	if again, _ := Diff(cur, cur, seen, model.Average); len(again) != 0 {
		t.Errorf("the same problem must not repeat: %+v", again)
	}
}

func TestFiltering(t *testing.T) {
	prev := snap()
	sup := prob("2", model.High)
	sup.Suppressed = true
	notes, _ := Diff(prev, snap(prob("1", model.Warning), sup), map[string]bool{}, model.Average)
	if len(notes) != 0 {
		t.Errorf("low severity and suppressed stay quiet: %+v", notes)
	}
	notes, _ = Diff(snap(prob("1", model.Warning), sup), snap(), map[string]bool{}, model.Average)
	if len(notes) != 0 {
		t.Errorf("their resolution stays quiet too: %+v", notes)
	}
}

func TestCapPerPoll(t *testing.T) {
	var many []model.Problem
	for i := 0; i < 10; i++ {
		many = append(many, prob(fmt.Sprint(i), model.High))
	}
	notes, _ := Diff(snap(), snap(many...), map[string]bool{}, model.Average)
	if len(notes) != 3 {
		t.Errorf("%d new", len(notes))
	}
	notes, _ = Diff(snap(many...), snap(), map[string]bool{}, model.Average)
	if len(notes) != 3 {
		t.Errorf("%d resolved", len(notes))
	}
}

func TestTruncatedPageProvesNothing(t *testing.T) {
	prev := snap(prob("1", model.High))
	cur := snap(prob("2", model.High))
	cur.Truncated = true
	notes, _ := Diff(prev, cur, map[string]bool{"1": true, "2": true}, model.Average)
	if len(notes) != 0 {
		t.Errorf("a problem off a full page is not resolved (D10): %+v", notes)
	}
}

func TestHostTransitions(t *testing.T) {
	host := func(id string, a model.Availability, e string) model.HostRow {
		return model.HostRow{ID: id, Name: "h" + id, Address: "10.0.0.1:10050", Availability: a, Error: e}
	}
	prev := &model.Snapshot{Hosts: []model.HostRow{host("1", model.Up, ""), host("2", model.Down, ""), host("3", model.Up, ""), host("4", model.Unknown, "")}}
	cur := &model.Snapshot{Hosts: []model.HostRow{host("1", model.Down, "agent gone"), host("2", model.Up, ""), host("3", model.Down, ""), host("4", model.Down, ""), host("5", model.Down, "")}}
	notes, _ := Diff(prev, cur, map[string]bool{}, model.Disaster)
	if len(notes) != 3 {
		t.Fatalf("%+v", notes)
	}
	if n := notes[0]; n.Kind != HostDown || n.Title != "Unreachable · h1" || n.Body != "agent gone" || !n.Urgent {
		t.Errorf("%+v", n)
	}
	if n := notes[1]; n.Kind != HostUp || n.Title != "Back online · h2" {
		t.Errorf("%+v", n)
	}
	if notes[2].Body != "10.0.0.1:10050" {
		t.Errorf("no error falls back to the address: %+v", notes[2])
	}
}
