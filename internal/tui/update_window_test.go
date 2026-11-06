package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdamigos/cream/internal/update"
)

func TestUpdateWindow(t *testing.T) {
	_, mgr := newModel(t)
	md := New(mgr, Options{Version: "v0.1.3", SkipIntro: true, UpdateCheck: true})
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40})

	md = send(t, md, updateMsg{&update.Result{Latest: "v0.1.3", Newer: false}}, key("U"))
	if md.modal != modalNone || !strings.Contains(md.toasts[len(md.toasts)-1].text, "up to date") {
		t.Fatal("U without a newer version should only say it's up to date")
	}

	md = send(t, md, updateMsg{&update.Result{Latest: "v0.1.4", Newer: true}})
	if !strings.Contains(plain(md), "v0.1.4 · U") {
		t.Fatal("badge doesn't point at U")
	}
	md = send(t, md, key("U"))
	v := plain(md)
	if md.modal != modalUpdate || !strings.Contains(v, "v0.1.4 is available") || !strings.Contains(v, "installed with") {
		t.Fatalf("update window:\n%s", v)
	}

	var opened string
	openURL = func(_, url string) error { opened = url; return nil }
	defer func() { openURL = openInBrowser }()
	md = send(t, md, key("o"))
	if opened != "https://github.com/gdamigos/cream/releases/tag/v0.1.4" || md.modal != modalNone {
		t.Fatalf("opened %q", opened)
	}

	// a tampered cache can't make cream open some other page
	md = send(t, md, updateMsg{&update.Result{Latest: "https://evil.example", Newer: true}}, key("U"), key("o"))
	if opened != "https://github.com/gdamigos/cream/releases/latest" {
		t.Fatalf("opened %q", opened)
	}
}
