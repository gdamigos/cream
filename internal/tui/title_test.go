package tui

import (
	"strings"
	"testing"
	"time"
)

func TestWindowTitle(t *testing.T) {
	md, mgr := newModel(t)
	mgr.Cfg.Active = "work"
	md = send(t, md, frameMsg(time.Now()))
	if md.title != "cream · work" {
		t.Fatalf("idle title %q", md.title)
	}
	if _, cmd := md.syncTitle(); cmd != nil {
		t.Fatal("unchanged title sent again")
	}

	md.busy, md.busyLabel = true, "switching to personal"
	first := md.windowTitle()
	if !strings.HasSuffix(first, " cream · switching to personal") {
		t.Fatalf("busy title %q", first)
	}
	md.frame += 3
	if md.windowTitle() == first {
		t.Fatal("busy title doesn't animate")
	}
	md.busy = false

	md.toast(1, "now using personal")
	if got := md.windowTitle(); got != "✓ cream · now using personal" {
		t.Fatalf("toast title %q", got)
	}
	md.toasts = nil

	md.modal, md.pend = modalConfirm, &pending{title: "Delete profile work?"}
	if got := md.windowTitle(); got != "cream · delete profile work?" {
		t.Fatalf("confirm title %q", got)
	}
	md.modal = modalHelp
	if got := md.windowTitle(); got != "cream · help" {
		t.Fatalf("help title %q", got)
	}
	md.modal = modalNone

	md.tab, md.dryRun = tabRules, true
	if got := md.windowTitle(); got != "cream · work · rules (dry run)" {
		t.Fatalf("tab title %q", got)
	}
	md.tab, md.dryRun = tabStatus, false

	mgr.Cfg.Lock.All = true
	if got := md.windowTitle(); got != "cream · locked" {
		t.Fatalf("locked title %q", got)
	}
}
