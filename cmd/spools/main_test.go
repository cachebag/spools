package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cachebag/spools/internal/adapter"
	"github.com/cachebag/spools/internal/bundle"
)

type fake struct{ sessions []adapter.Session }

func (f *fake) Name() string                          { return "fake" }
func (f *fake) Detect() bool                          { return true }
func (f *fake) Running() bool                         { return false }
func (f *fake) List() ([]adapter.Session, error)      { return f.sessions, nil }
func (f *fake) KnownProjects() ([]string, error)      { return nil, nil }
func (f *fake) Export(string) (*bundle.Bundle, error) { return nil, nil }
func (f *fake) Import(*bundle.Bundle, adapter.ImportOptions) (*adapter.ImportResult, error) {
	return nil, nil
}

func TestListSessionsEmptyIsNotNull(t *testing.T) {
	got, err := listSessions(&fake{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if out, _ := json.Marshal(got); string(out) != "[]" {
		t.Fatalf("got %s", out)
	}
}

func TestPullAllKeepsGoing(t *testing.T) {
	sessions := []adapter.Session{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	var tried []string
	var stderr bytes.Buffer
	err := pullAll(sessions, &stderr, func(id string) error {
		tried = append(tried, id)
		if id == "2" {
			return errors.New("boom")
		}
		return nil
	})
	if len(tried) != 3 {
		t.Fatalf("tried %v, want all three", tried)
	}
	if err == nil || err.Error() != "1 of 3 sessions failed" {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(stderr.String(), "2: boom") {
		t.Fatalf("stderr %q", stderr.String())
	}

	if err := pullAll(sessions, &stderr, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestListSessions(t *testing.T) {
	a := &fake{sessions: []adapter.Session{
		{ID: "1", Project: "/code/relay"},
		{ID: "2", Project: "/code/other"},
	}}
	for _, tc := range []struct {
		filter string
		want   int
	}{
		{"", 2},
		{"relay", 1},
		{"nothing", 0},
	} {
		got, err := listSessions(a, tc.filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != tc.want {
			t.Fatalf("filter %q: got %d sessions, want %d", tc.filter, len(got), tc.want)
		}
		out, _ := json.Marshal(got)
		if tc.want == 0 && string(out) != "[]" {
			t.Fatalf("empty list should encode as [], got %s", out)
		}
	}
}

func TestSessionJSONRoundTrip(t *testing.T) {
	in := adapter.Session{ID: "1", Title: "t", Project: "/code/x", UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out adapter.Session
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}
