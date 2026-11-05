package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// dumps frames to $CREAM_SNAPSHOT_DIR, skipped when unset
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("CREAM_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("set CREAM_SNAPSHOT_DIR to dump frames")
	}
	md, mgr := newModel(t)
	mgr.Import(ctxBG(), "work", nil, false, nil)
	md = send(t, md, tea.WindowSizeMsg{Width: 110, Height: 32}, statusMsg(mgr.StatusFor(ctxBG(), nil)))
	frames := map[string]string{}
	frames["main"] = plain(md)
	md2 := send(t, md, key("down"))
	frames["main-work"] = plain(md2)
	frames["help"] = plain(send(t, md, key("?")))
	frames["new"] = plain(send(t, md, key("n")))
	frames["import"] = plain(send(t, md, key("i")))
	frames["rules"] = plain(send(t, md, key("2")))
	intro := New(mgr, Options{Version: "v0.1.0"})
	intro.start = time.Now().Add(-1700 * time.Millisecond)
	intro = send(t, intro, tea.WindowSizeMsg{Width: 110, Height: 32})
	frames["intro"] = plain(intro)
	for k, v := range frames {
		os.WriteFile(dir+"/"+k+".txt", []byte(strings.TrimRight(v, " \n")), 0o644)
	}
}
