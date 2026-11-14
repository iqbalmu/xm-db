package commands

import (
	"testing"

	"github.com/iqbalmu/dbm/internal/state"
)

func TestFilterByGroup(t *testing.T) {
	dbs := []*state.Database{
		{Name: "work-api", Group: "work"},
		{Name: "work-billing", Group: "work"},
		{Name: "personal-blog", Group: "personal"},
		{Name: "sandbox"},
	}

	if got := len(filterByGroup(dbs, "")); got != 4 {
		t.Errorf("empty filter kept %d, want 4", got)
	}
	if got := len(filterByGroup(dbs, "work")); got != 2 {
		t.Errorf("work filter kept %d, want 2", got)
	}
	if got := len(filterByGroup(dbs, "personal")); got != 1 {
		t.Errorf("personal filter kept %d, want 1", got)
	}
	if got := len(filterByGroup(dbs, "global")); got != 1 {
		t.Errorf("global filter kept %d, want 1", got)
	}
	if got := filterByGroup(dbs, "global")[0].Name; got != "sandbox" {
		t.Errorf("global filter returned %q, want sandbox", got)
	}
	if got := len(filterByGroup(dbs, "nope")); got != 0 {
		t.Errorf("unknown filter kept %d, want 0", got)
	}
}

func TestGroupLabel(t *testing.T) {
	if got := groupLabel(""); got != "global" {
		t.Errorf("groupLabel(\"\") = %q, want global", got)
	}
	if got := groupLabel("work"); got != "work" {
		t.Errorf("groupLabel(work) = %q, want work", got)
	}
}
