package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdamigos/cream/internal/core"
	"github.com/gdamigos/cream/internal/usage"
)

func TestWindowLabel(t *testing.T) {
	for in, want := range map[string]string{"5h": "5-hour", "7d": "weekly", "7d opus": "weekly Opus", "30d": "30-day", "90m": "90-minute", "limit": "limit"} {
		if got := windowLabel(in); got != want {
			t.Errorf("windowLabel(%q) = %q, want %q", in, got, want)
		}
	}
	reset := time.Date(2026, 11, 9, 15, 0, 0, 0, time.Local)
	if got := ansi.Strip(windowText("5h", 100, reset, reset.Add(-time.Hour))); got != "5-hour limit reached · resets 15:00" {
		t.Errorf("limit reached text %q", got)
	}
}

func TestUsageSuggestAndCard(t *testing.T) {
	md, mgr := newModel(t)
	for _, n := range []string{"work", "personal"} {
		if _, err := mgr.Import(ctxBG(), n, []string{"claude"}, false, nil); err != nil {
			t.Fatal(err)
		}
	}
	mgr.Cfg.Active = "work"
	mgr.Cfg.Profiles["personal"].Accounts["claude"].Imported = time.Now().Add(-60 * 24 * time.Hour)
	md = send(t, md, tea.WindowSizeMsg{Width: 140, Height: 45}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})

	reset := time.Now().Add(2 * time.Hour)
	us := usageMsg{
		{Profile: "personal", Adapter: "claude", Name: "Claude Code", Result: &usage.Result{Windows: []usage.Window{{Name: "5h", Used: 20}}}},
		{Profile: "work", Adapter: "claude", Name: "Claude Code", Live: true,
			Result: &usage.Result{Limited: true, Windows: []usage.Window{{Name: "5h", Used: 100, ResetsAt: reset}}}},
	}
	md = send(t, md, us)
	if len(md.suggest) != 1 || md.suggest[0].To != "personal" {
		t.Fatalf("suggestions %+v", md.suggest)
	}
	if n := len(md.toasts); n == 0 || !strings.Contains(md.toasts[n-1].text, "S switches it to personal") {
		t.Fatalf("no limit toast, toasts %+v", md.toasts)
	}
	// the same limit again doesn't toast twice
	before := len(md.toasts)
	if md = send(t, md, us); len(md.toasts) != before {
		t.Fatal("the same limit was toasted again")
	}
	if !strings.Contains(plain(md), "S switch Claude Code to personal") {
		t.Fatalf("footer has no S hint:\n%s", plain(md))
	}

	// the profile card of personal, selected from the start, shows its usage and the stale login
	if md.selected().Name != "personal" {
		t.Fatal("personal should be selected")
	}
	screen := plain(md)
	if !strings.Contains(screen, "20% of the 5-hour limit used") || !strings.Contains(screen, "plan usage from Anthropic and OpenAI") ||
		!strings.Contains(screen, "last refreshed 60 days ago") {
		t.Fatalf("card lacks usage or stale row:\n%s", screen)
	}

	// S previews switching only Claude Code to personal
	md = send(t, md, key("S"))
	if md.modal != modalConfirm || md.pend.profile != "personal" || len(md.pend.only) != 1 || md.pend.only[0] != "claude" {
		t.Fatalf("S should preview the claude switch, got %+v", md.pend)
	}

	// once nothing is limited S only says so
	md = send(t, md, key("esc"), usageMsg([]core.Usage{}))
	if md = send(t, md, key("S")); md.modal != modalNone {
		t.Fatal("S opened a window with nothing at its limit")
	}
}
